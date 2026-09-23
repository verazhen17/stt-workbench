# Batch Faster Whisper transcription

Install the Faster Whisper dependency in the Python environment that has access to the model files:

```sh
python3 -m pip install -r scripts/requirements.txt
```

Transcribe every stream directory under `backend/data/samples`:

```sh
python3 scripts/batch_transcribe.py \
  --preset-name "large-v3 nightly run" \
  --model /models/faster-whisper-large-v3 \
  --beam-size 5 \
  --temperature 0.2 \
  --concurrency 2
```

Select specific stream IDs by adding `--stream-ids 12345612 12345613`. Without stream IDs, the script processes every immediate stream directory under the samples root. Use `--samples-root PATH` to select another samples directory.

The script creates one UUID preset per run in `presets/`. It adds a stream ID to that preset as soon as every WAV in that stream finishes successfully, without waiting for other streams. Each successful WAV writes `{startTs}_{index}_{preset_id}.json` next to the source file; each result includes `vod_id` and integer millisecond `start_ms`/`end_ms` fields matching the workbench backend. A partial stream can have successful result files while remaining absent from the preset's successful `stream_ids` list.

`--concurrency` controls the maximum number of simultaneous WAV transcriptions and defaults to 1. Each worker loads its own model instance once and reuses it, so higher concurrency also uses more GPU memory. WAV names that would overwrite the same `{startTs}_{index}_{preset_id}.json` output are rejected before inference. Each output is written through a unique temporary file and atomically renamed into place.

The script prints a JSON report and exits with status 1 if any WAV fails.

The run label (`--preset-name`) and concurrency are included in the final report, not the strict backend preset manifest. Completed streams remain indexed if the process is interrupted later. Resume and automatic retries are not supported; rerunning creates a new preset.
