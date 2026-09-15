#!/usr/bin/env python3
"""Batch-transcribe WAV files in one or more stream directories."""

from __future__ import annotations

import argparse
from concurrent.futures import ThreadPoolExecutor, as_completed
import json
from pathlib import Path
import re
import sys
import threading
from typing import Any, Callable
import uuid
from datetime import datetime, timezone

try:
    from whisper import transcribe_audio
except ImportError:
    from .whisper import transcribe_audio


STREAM_ID_PATTERN = re.compile(r"^[A-Za-z0-9-]+$")
DIGITS_PATTERN = re.compile(r"^\d+$")


def utc_now() -> str:
    return datetime.now(timezone.utc).isoformat(timespec="seconds").replace("+00:00", "Z")


def atomic_json_write(path: Path, document: dict[str, Any]) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    temporary_path = path.with_name(f".{path.name}.{uuid.uuid4().hex}.tmp")
    try:
        temporary_path.write_text(
            json.dumps(document, ensure_ascii=False, indent=2) + "\n", encoding="utf-8"
        )
        # Publish a complete document so concurrent workbench reads never see partial JSON.
        temporary_path.replace(path)
    finally:
        temporary_path.unlink(missing_ok=True)


def parse_vod_id(wav_path: Path) -> str | None:
    parts = wav_path.stem.split("_", 2)
    if len(parts) != 3 or not DIGITS_PATTERN.fullmatch(parts[0]) or not DIGITS_PATTERN.fullmatch(parts[1]):
        return None
    if not parts[2]:
        return None
    return f"{parts[0]}_{parts[1]}"


def discover_streams(samples_root: Path, requested_ids: list[str] | None) -> list[Path]:
    if requested_ids:
        stream_dirs = []
        for stream_id in dict.fromkeys(requested_ids):
            if not STREAM_ID_PATTERN.fullmatch(stream_id):
                raise ValueError(f"invalid stream ID: {stream_id}")
            stream_dir = samples_root / stream_id
            if not stream_dir.is_dir():
                raise ValueError(f"stream directory not found: {stream_id}")
            stream_dirs.append(stream_dir)
        return stream_dirs

    return sorted(
        (entry for entry in samples_root.iterdir() if entry.is_dir() and entry.name != "presets"),
        key=lambda path: path.name,
    )


def make_model(model_name: str, device: str, compute_type: str) -> Any:
    try:
        from faster_whisper import WhisperModel
    except ImportError as error:
        raise RuntimeError(
            "faster-whisper is not installed; install scripts/requirements.txt"
        ) from error
    return WhisperModel(model_name, device=device, compute_type=compute_type)


def run_batch(
    args: argparse.Namespace,
    model_factory: Callable[[str, str, str], Any] = make_model,
) -> dict[str, Any]:
    if args.concurrency < 1:
        raise ValueError("concurrency must be greater than zero")
    samples_root = args.samples_root.resolve()
    if not samples_root.is_dir():
        raise ValueError(f"samples directory not found: {samples_root}")

    stream_dirs = discover_streams(samples_root, args.stream_ids)
    if not stream_dirs:
        raise ValueError("no stream directories found")

    preset_id = str(uuid.uuid4())
    created_at = utc_now()
    preset_path = samples_root / "presets" / f"{preset_id}.json"
    params: dict[str, Any] = {
        "temperature": getattr(args, "temperature", 0.0),
        "beam_size": getattr(args, "beam_size", 5),
        "vad_filter": getattr(args, "vad_filter", True),
        "device": getattr(args, "device", "cpu"),
        "compute_type": getattr(args, "compute_type", "int8"),
    }
    if getattr(args, "language", ""):
        params["language"] = args.language
    prompt = getattr(args, "prompt", "") or getattr(args, "initial_prompt", "")
    if prompt:
        params["initial_prompt"] = prompt

    preset: dict[str, Any] = {
        "preset_id": preset_id,
        "name": args.preset_name,
        "model": {"name": args.model, "params": params},
        "stream_ids": [],
        "created_at": created_at,
        "updated_at": created_at,
    }
    atomic_json_write(preset_path, preset)

    report: dict[str, Any] = {"preset_id": preset_id, "preset_name": args.preset_name, "concurrency": args.concurrency, "items": []}
    jobs: list[tuple[Path, dict[str, Any], Path]] = []
    output_counts: dict[Path, int] = {}
    for stream_dir in stream_dirs:
        wav_files = sorted(stream_dir.glob("*.wav"), key=lambda path: path.name)
        if not wav_files:
            report["items"].append(
                {"stream_id": stream_dir.name, "status": "failed", "error": "no WAV files found"}
            )
            continue

        for wav_path in wav_files:
            vod_id = parse_vod_id(wav_path)
            item: dict[str, Any] = {"stream_id": stream_dir.name, "wav": wav_path.name}
            if vod_id is None:
                item.update(status="failed", error="WAV name must be {startTs}_{index}_{filename}.wav")
                report["items"].append(item)
                continue
            item["vod_id"] = vod_id
            output_path = stream_dir / f"{vod_id}_{preset_id}.json"
            output_counts[output_path] = output_counts.get(output_path, 0) + 1
            report["items"].append(item)
            jobs.append((wav_path, item, output_path))

    runnable_jobs: list[tuple[Path, dict[str, Any], Path]] = []
    for job in jobs:
        wav_path, item, output_path = job
        if output_counts[output_path] > 1:
            item.update(status="failed", error="multiple WAV files map to the same STT output filename")
        else:
            runnable_jobs.append((wav_path, item, output_path))

    # Count every input, including validation failures: a partial stream must stay hidden.
    remaining = {stream_dir.name: 0 for stream_dir in stream_dirs}
    for item in report["items"]:
        remaining[item["stream_id"]] += 1

    # Each thread owns and reuses its model; increasing concurrency costs model memory.
    worker_state = threading.local()

    def process_job(job: tuple[Path, dict[str, Any], Path]) -> None:
        wav_path, item, output_path = job
        try:
            if not hasattr(worker_state, "model"):
                worker_state.model = model_factory(args.model, getattr(args, "device", "cpu"), getattr(args, "compute_type", "int8"))
            
            segments, detected_language, error = transcribe_audio(
                file_path=str(wav_path),
                model=worker_state.model,
                chunk_length=getattr(args, "chunk_length", 150),
                prompt=prompt,
                language=getattr(args, "language", ""),
                beam_size=getattr(args, "beam_size", 5),
                vad_filter=getattr(args, "vad_filter", True),
            )
            if error:
                item.update(status="failed", error=error)
                return

            result = {
                "preset_id": preset_id,
                "stream_id": item["stream_id"],
                "vod_id": item["vod_id"],
                "language": detected_language,
                "created_at": utc_now(),
                "segments": segments,
            }
            atomic_json_write(output_path, result)
            item["status"] = "completed"
            if detected_language:
                item["language"] = detected_language
        except Exception as err:
            item.update(status="failed", error=str(err))

    if runnable_jobs:
        with ThreadPoolExecutor(max_workers=args.concurrency) as executor:
            futures = {executor.submit(process_job, job): job[1] for job in runnable_jobs}
            for future in as_completed(futures):
                future.result()
                item = futures[future]
                if item["status"] != "completed":
                    continue
                stream_id = item["stream_id"]
                remaining[stream_id] -= 1
                if remaining[stream_id] == 0:
                    # Only the coordinator writes the shared index, after all result files
                    # for this stream exist. Other streams need not finish first.
                    preset["stream_ids"].append(stream_id)
                    preset["stream_ids"].sort()
                    preset["updated_at"] = utc_now()
                    atomic_json_write(preset_path, preset)

    return report


def build_parser() -> argparse.ArgumentParser:
    default_samples_root = Path(__file__).resolve().parents[1] / "backend" / "data" / "samples"
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--samples-root", type=Path, default=default_samples_root)
    parser.add_argument("--preset-name", required=True, help="Human-readable name for this run's preset")
    parser.add_argument("--model", required=True, help="Faster Whisper model name or model directory")
    parser.add_argument(
        "--stream-ids",
        nargs="*",
        help="Stream IDs to process; omit or pass no IDs to process every stream directory",
    )
    parser.add_argument("--temperature", type=float, default=0.0)
    parser.add_argument("--beam-size", type=int, default=5)
    parser.add_argument("--vad-filter", action=argparse.BooleanOptionalAction, default=True)
    parser.add_argument("--language", default="")
    parser.add_argument("--prompt", default="", help="Initial prompt (alias for --initial-prompt)")
    parser.add_argument("--initial-prompt", default="")
    parser.add_argument("--chunk-length", "--chunk_length", type=int, default=150, dest="chunk_length")
    parser.add_argument("--device", default="cuda")
    parser.add_argument("--compute-type", default="float16")
    parser.add_argument(
        "--concurrency",
        type=int,
        default=1,
        help="Maximum simultaneous transcriptions; each worker loads its own model",
    )
    return parser


def main(argv: list[str] | None = None) -> int:
    parser = build_parser()
    args = parser.parse_args(argv)
    if args.beam_size < 1:
        parser.error("--beam-size must be greater than zero")
    if args.concurrency < 1:
        parser.error("--concurrency must be greater than zero")
    try:
        report = run_batch(args)
    except (OSError, ValueError, RuntimeError) as error:
        print(json.dumps({"error": str(error)}, ensure_ascii=False), file=sys.stderr)
        return 1
    print(json.dumps(report, ensure_ascii=False, indent=2))
    return 1 if report.get("error") or any(item["status"] == "failed" for item in report["items"]) else 0


if __name__ == "__main__":
    raise SystemExit(main())
