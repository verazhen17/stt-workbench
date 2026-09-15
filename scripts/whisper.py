import argparse
import json
import os
import shutil
import sys
import time
import uuid

try:
    from pydub import AudioSegment
except ImportError:
    AudioSegment = None


def ms_to_time_format(seconds: float) -> str:
    """Format seconds into HH:MM:SS.mmm (matching stt-service TranscriptionOutput)."""
    return time.strftime("%H:%M:%S", time.gmtime(seconds)) + (".%03d" % int(round((seconds % 1) * 1000)))


def slice_audio(input_path: str, output_dir: str, chunk_length_ms: int) -> list[str]:
    if AudioSegment is None:
        raise RuntimeError("pydub is not installed; please install pydub to enable audio slicing")
    audio = AudioSegment.from_file(input_path)
    os.makedirs(output_dir, exist_ok=True)
    file_list = []
    for i, start in enumerate(range(0, len(audio), chunk_length_ms)):
        chunk = audio[start : start + chunk_length_ms]
        file_path = f"{output_dir}/chunk_{i:03d}.wav"
        chunk.export(file_path, format="wav")
        file_list.append(file_path)
    return file_list


def segment_transcribe(file_path: str, model, transcribe_kwargs: dict):
    err_msg = ""
    try:
        results, info = model.transcribe(file_path, **transcribe_kwargs)
    except ValueError as ve:
        if "max() arg is an empty sequence" in str(ve):
            # No valid speech detected. Please check the audio file or VAD settings
            err_msg += "Error: " + file_path + ": No valid speech detected"
            return [], None, err_msg
        else:
            # Other ValueError occurred
            err_msg += "Error: " + file_path + ": ValueError occurred - " + str(ve)
            return [], None, err_msg
    except Exception as e:
        # An unexpected error occurred
        err_msg += "Error: " + file_path + ": Unexpected error occurred - " + str(e)
        return [], None, err_msg
    return results, info, err_msg


def transcribe_audio(
    file_path: str,
    model,
    chunk_length: int = 150,
    prompt: str = "",
    language: str = "",
    beam_size: int = 5,
    vad_filter: bool = True,
) -> tuple[list[dict], str, str]:
    """Transcribe audio file matching stt-service whisper.py logic.

    Returns (segments, detected_language, error).
    Each segment is formatted as:
    {
        "timestamps": {
            "from": "00:00:01.200",
            "to": "00:00:02.345"
        },
        "text": "..."
    }
    """
    chunk_length_ms = chunk_length * 60 * 1000
    tmp_segment_output_dir = f"/tmp/sliced_audio/{uuid.uuid4().hex}"

    transcribe_kwargs = {"beam_size": beam_size, "vad_filter": vad_filter}
    if language:
        transcribe_kwargs["language"] = language
    if prompt:
        transcribe_kwargs["initial_prompt"] = prompt

    subtitles_all = []
    error = ""
    detected_language = language or ""
    last_info = None

    try:
        # Determine if the audio needs to be sliced
        audio_chunks = [file_path]
        if AudioSegment is not None and os.path.exists(file_path):
            try:
                audio = AudioSegment.from_file(file_path)
                if len(audio) > chunk_length_ms:
                    audio_chunks = slice_audio(file_path, tmp_segment_output_dir, chunk_length_ms)
            except Exception:
                # If audio loading fails (e.g. non-audio test mock), fallback to direct file
                audio_chunks = [file_path]

        for i, audio_chunk in enumerate(audio_chunks):
            segment_start_time = i * chunk_length_ms / 1000

            subtitles, info, err_msg = segment_transcribe(audio_chunk, model, transcribe_kwargs)
            subtitles = list(subtitles) if subtitles is not None else []
            if info is not None:
                last_info = info
                if hasattr(info, "language") and info.language:
                    detected_language = info.language

            if len(subtitles) != 0:
                if segment_start_time != 0:
                    for segment in subtitles:
                        segment.start += segment_start_time
                        segment.end += segment_start_time
                subtitles_all.extend(subtitles)
            if err_msg:
                error += f"{';' if error else ''}{err_msg}"
    finally:
        shutil.rmtree(tmp_segment_output_dir, ignore_errors=True)

    formatted_segments = [
        {
            "timestamps": {
                "from": ms_to_time_format(segment.start),
                "to": ms_to_time_format(segment.end),
            },
            "text": segment.text,
        }
        for segment in subtitles_all
    ]

    return formatted_segments, detected_language, error


def output_json(model_name: str, language: str, segments: list[dict], error: str):
    transcription = {
        "model": model_name,
        "language": language,
        "transcription": segments,
        "error": error,
    }
    print(json.dumps(transcription, ensure_ascii=False, indent=4))


def main():
    from faster_whisper import WhisperModel

    parser = argparse.ArgumentParser(description="Transcribe audio files using WhisperModel.")
    parser.add_argument("--model", type=str, default="large-v3", help="Specify the model name.")
    parser.add_argument("--language", type=str, default="", help="Specify the language for transcription.")
    parser.add_argument("--prompt", type=str, default="", help="Specify the initial prompt for transcription.")
    parser.add_argument("--chunk_length", type=int, default=150, help="Specify the chunk length for transcription.")
    parser.add_argument("--device", type=str, default="cuda" if os.getenv("USE_CUDA") == "1" else "cpu")
    parser.add_argument("--compute_type", type=str, default="float16" if os.getenv("USE_CUDA") == "1" else "int8")
    parser.add_argument("file_path", type=str, nargs="?", help="Path to the audio file.")

    args, unknown = parser.parse_known_args()

    if args.file_path is None:
        for arg in unknown:
            if not arg.startswith("--"):
                args.file_path = arg
                unknown.remove(arg)
                break

    if args.file_path is None:
        parser.error("The file_path argument is required.")
        sys.exit(1)

    model = WhisperModel(args.model, device=args.device, compute_type=args.compute_type)
    segments, language, error = transcribe_audio(
        file_path=args.file_path,
        model=model,
        chunk_length=args.chunk_length,
        prompt=args.prompt,
        language=args.language,
    )
    output_json(args.model, language, segments, error)


if __name__ == "__main__":
    main()
