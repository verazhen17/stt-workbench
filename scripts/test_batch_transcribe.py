import json
from pathlib import Path
import tempfile
import threading
import time
from types import SimpleNamespace
import unittest
from unittest.mock import patch
from argparse import Namespace

from batch_transcribe import atomic_json_write, build_parser, run_batch
from whisper import is_blacklisted_transcription, ms_to_time_format, transcribe_audio


class FakeModel:
    def __init__(self, detected_language="zh"):
        self.detected_language = detected_language

    def transcribe(self, wav_path, **_options):
        if "fails" in wav_path:
            raise RuntimeError("synthetic inference failure")
        return [SimpleNamespace(start=1.2, end=2.345, text="晚安")], SimpleNamespace(language=self.detected_language)


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

    def args(self, stream_ids=None, language="zh"):
        return Namespace(
            samples_root=self.root,
            preset_name="nightly batch",
            model="whisper-large-v3",
            stream_ids=stream_ids,
            temperature=0.2,
            beam_size=5,
            vad_filter=True,
            language=language,
            prompt="直播",
            initial_prompt="直播",
            chunk_length=5,
            device="cpu",
            compute_type="int8",
            concurrency=1,
        )

    def test_selected_streams_share_one_preset_and_write_timestamps_and_language(self):
        self.add_wav("123", "100_000_clip.wav")
        self.add_wav("123", "100_001_second.wav")
        self.add_wav("456", "200_000_clip.wav")
        self.add_wav("789", "300_000_clip.wav")

        report = run_batch(self.args(["123", "456"], language=""), lambda *_: FakeModel(detected_language="ja"))

        self.assertEqual(len(report["items"]), 3)
        preset_path = self.root / "presets" / f"{report['preset_id']}.json"
        preset = json.loads(preset_path.read_text(encoding="utf-8"))
        self.assertEqual(report["preset_name"], "nightly batch")
        self.assertEqual(set(preset), {"preset_id", "name", "model", "stream_ids", "created_at", "updated_at"})
        self.assertEqual(preset["name"], "nightly batch")
        self.assertEqual(preset["stream_ids"], ["123", "456"])
        self.assertEqual(preset["model"]["params"]["beam_size"], 5)
        result_path = self.root / "123" / f"100_000_{report['preset_id']}.json"
        result = json.loads(result_path.read_text(encoding="utf-8"))
        self.assertEqual(result["language"], "ja")
        self.assertEqual(result["segments"][0]["timestamps"]["from"], "00:00:01.199")
        self.assertEqual(result["segments"][0]["timestamps"]["to"], "00:00:02.345")
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
                return [SimpleNamespace(start=0, end=1, text="ok")], SimpleNamespace(language="zh")

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

    def test_whisper_transcribe_audio_empty_speech_error_handling(self):
        class EmptySequenceModel:
            def transcribe(self, *_args, **_kwargs):
                raise ValueError("max() arg is an empty sequence")

        segments, lang, err = transcribe_audio("dummy.wav", EmptySequenceModel())
        self.assertEqual(segments, [])
        self.assertIn("No valid speech detected", err)

    def test_whisper_filters_known_hallucination_phrases(self):
        class HallucinationModel:
            def transcribe(self, *_args, **_kwargs):
                return [
                    SimpleNamespace(start=0, end=1, text="正常內容"),
                    SimpleNamespace(start=1, end=2, text="前文 字幕由 Amara.org 社群提供"),
                    SimpleNamespace(start=2, end=3, text="优优独播剧场——YoYo Television Series Exclusive"),
                ], SimpleNamespace(language="zh")

        segments, _, _ = transcribe_audio("dummy.wav", HallucinationModel())
        self.assertEqual([segment["text"] for segment in segments], ["正常內容"])

    def test_blacklist_matches_substrings_only(self):
        self.assertTrue(is_blacklisted_transcription("請不吝點贊訂閱轉發打賞支持明鏡與點點欄目。"))
        self.assertTrue(is_blacklisted_transcription("中文字幕:CaptionCube"))
        self.assertFalse(is_blacklisted_transcription("Thank you"))

    def test_ms_to_time_format(self):
        self.assertEqual(ms_to_time_format(0), "00:00:00.000")
        self.assertEqual(ms_to_time_format(1.2), "00:00:01.199")
        self.assertEqual(ms_to_time_format(1.234), "00:00:01.234")
        self.assertEqual(ms_to_time_format(65.5), "00:01:05.500")
        self.assertEqual(ms_to_time_format(3661.025), "01:01:01.025")
        self.assertEqual(ms_to_time_format(1.2349), "00:00:01.234")
        self.assertEqual(ms_to_time_format(1.9999), "00:00:01.999")

    def test_default_chunk_length_matches_service_configuration(self):
        args = build_parser().parse_args(["--preset-name", "test", "--model", "large-v3"])
        self.assertEqual(args.chunk_length, 5)


if __name__ == "__main__":
    unittest.main()
