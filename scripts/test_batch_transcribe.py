import json
import tempfile
import threading
import time
import unittest
from argparse import Namespace
from pathlib import Path
from types import SimpleNamespace
from unittest.mock import patch

from batch_transcribe import atomic_json_write, run_batch


class FakeModel:
    def transcribe(self, wav_path, **_options):
        if "fails" in wav_path:
            raise RuntimeError("synthetic inference failure")
        return [SimpleNamespace(start=1.2, end=2.345, text="晚安")], None


class BatchTranscribeTests(unittest.TestCase):
    def setUp(self):
        self.temporary_directory = tempfile.TemporaryDirectory()
        self.root = Path(self.temporary_directory.name)
        (self.root / "presets").mkdir()
        for stream_id in ("123", "456", "789"):
            (self.root / stream_id).mkdir()

    def tearDown(self):
        self.temporary_directory.cleanup()

    def add_wav(self, stream_id, name):
        (self.root / stream_id / name).write_bytes(b"test")

    def args(self, stream_ids=None):
        return Namespace(
            samples_root=self.root,
            preset_name="nightly batch",
            model="whisper-large-v3",
            stream_ids=stream_ids,
            temperature=0.2,
            beam_size=5,
            vad_filter=True,
            language="zh",
            initial_prompt="直播",
            device="cpu",
            compute_type="int8",
            concurrency=1,
        )

    def test_selected_streams_share_one_preset_and_write_milliseconds(self):
        self.add_wav("123", "100_000_clip.wav")
        self.add_wav("123", "100_001_second.wav")
        self.add_wav("456", "200_000_clip.wav")
        self.add_wav("789", "300_000_clip.wav")

        report = run_batch(self.args(["123", "456"]), lambda *_: FakeModel())

        self.assertEqual(len(report["items"]), 3)
        preset_path = self.root / "presets" / f"{report['preset_id']}.json"
        preset = json.loads(preset_path.read_text(encoding="utf-8"))
        self.assertEqual(report["preset_name"], "nightly batch")
        self.assertEqual(set(preset), {"preset_id", "model", "stream_ids", "created_at", "updated_at"})
        self.assertEqual(preset["stream_ids"], ["123", "456"])
        self.assertEqual(preset["model"]["params"]["beam_size"], 5)
        result_path = self.root / "123" / f"100_000_{report['preset_id']}.json"
        result = json.loads(result_path.read_text(encoding="utf-8"))
        self.assertEqual(result["segments"][0]["start_ms"], 1200)
        self.assertEqual(result["segments"][0]["end_ms"], 2345)
        self.assertEqual(result["segments"][0]["text"], "晚安")
        self.assertEqual(result["vod_id"], "100_000")
        self.assertFalse((self.root / "789" / f"300_000_{report['preset_id']}.json").exists())

    def test_only_fully_successful_streams_are_added_to_preset(self):
        self.add_wav("123", "100_000_ok.wav")
        self.add_wav("123", "100_001_fails.wav")
        self.add_wav("456", "200_000_ok.wav")

        report = run_batch(self.args(), lambda *_: FakeModel())

        preset = json.loads((self.root / "presets" / f"{report['preset_id']}.json").read_text())
        self.assertEqual(preset["stream_ids"], ["456"])
        self.assertEqual(
            [item["status"] for item in report["items"][:3]],
            ["completed", "failed", "completed"],
        )
        self.assertTrue((self.root / "123" / f"100_000_{report['preset_id']}.json").exists())

    def test_concurrency_is_bounded_and_each_worker_owns_a_model(self):
        for index in range(8):
            self.add_wav("123", f"100_{index:03d}_clip.wav")
        state = {"active": 0, "maximum": 0, "models": 0}
        state_lock = threading.Lock()

        class SlowModel:
            def transcribe(self, _wav_path, **_options):
                with state_lock:
                    state["active"] += 1
                    state["maximum"] = max(state["maximum"], state["active"])
                time.sleep(0.03)
                with state_lock:
                    state["active"] -= 1
                return [SimpleNamespace(start=0, end=1, text="ok")], None

        def factory(*_args):
            with state_lock:
                state["models"] += 1
            return SlowModel()

        args = self.args(["123"])
        args.concurrency = 3
        report = run_batch(args, factory)

        self.assertTrue(all(item["status"] == "completed" for item in report["items"]))
        self.assertGreaterEqual(state["maximum"], 2)
        self.assertLessEqual(state["maximum"], 3)
        self.assertLessEqual(state["models"], 3)
        self.assertEqual(state["models"], state["maximum"])

    def test_duplicate_output_names_are_rejected_before_transcribing(self):
        self.add_wav("123", "100_000_first.wav")
        self.add_wav("123", "100_000_second.wav")
        calls = []

        def factory(*_args):
            calls.append(True)
            return FakeModel()

        report = run_batch(self.args(["123"]), factory)

        self.assertEqual(calls, [])
        self.assertEqual([item["status"] for item in report["items"]], ["failed", "failed"])
        self.assertEqual(json.loads((self.root / "presets" / f"{report['preset_id']}.json").read_text())["stream_ids"], [])

    def test_completed_stream_is_published_while_another_stream_is_running(self):
        self.add_wav("123", "100_000_clip.wav")
        self.add_wav("456", "200_000_clip.wav")
        published = threading.Event()
        observed = []

        def write_and_signal(path, document):
            atomic_json_write(path, document)
            if document.get("stream_ids") == ["123"]:
                published.set()

        class WaitingModel(FakeModel):
            def transcribe(self, wav_path, **options):
                if Path(wav_path).parent.name == "456":
                    # The other stream must become visible before this inference finishes.
                    observed.append(published.wait(timeout=5))
                return super().transcribe(wav_path, **options)

        args = self.args(["123", "456"])
        args.concurrency = 2
        with patch("batch_transcribe.atomic_json_write", side_effect=write_and_signal):
            report = run_batch(args, lambda *_: WaitingModel())
        self.assertEqual(observed, [True])
        self.assertTrue(all(item["status"] == "completed" for item in report["items"]))

    def test_lazy_inference_failure_does_not_publish_stream(self):
        self.add_wav("123", "100_000_clip.wav")

        class LazyFailureModel:
            def transcribe(self, *_args, **_options):
                def segments():
                    yield SimpleNamespace(start=0, end=1, text="partial")
                    raise RuntimeError("inference failed during iteration")
                return segments(), None

        report = run_batch(self.args(["123"]), lambda *_: LazyFailureModel())
        self.assertEqual(report["items"][0]["status"], "failed")
        self.assertEqual(list((self.root / "123").glob("*.json")), [])
        preset = json.loads((self.root / "presets" / f"{report['preset_id']}.json").read_text())
        self.assertEqual(preset["stream_ids"], [])


if __name__ == "__main__":
    unittest.main()
