export interface MediaPlayer {
  attach(element: HTMLVideoElement): void;
  load(source: string): void;
  play(): Promise<void>;
  pause(): void;
  seekTo(seconds: number): void;
  destroy(): void;
}

type MpegtsModule = typeof import("mpegts.js")["default"];

export class FlvMediaPlayer implements MediaPlayer {
  private readonly mpegts: MpegtsModule;
  private readonly onError: (message: string) => void;
  private player: ReturnType<MpegtsModule["createPlayer"]> | undefined;
  private element: HTMLVideoElement | undefined;

  constructor(mpegts: MpegtsModule, onError: (message: string) => void) {
    this.mpegts = mpegts;
    this.onError = onError;
  }

  private readonly handlePlayerError = (type: string, detail: string): void => {
    const hint = detail === this.mpegts.ErrorDetails.MEDIA_CODEC_UNSUPPORTED ||
      detail === this.mpegts.ErrorDetails.MEDIA_MSE_ERROR
      ? " H.265 playback requires browser and OS support for HEVC through Media Source Extensions."
      : "";
    this.onError(`Unable to play the VOD (${type}: ${detail}).${hint}`);
  };

  private readonly handleMediaError = (): void => {
    const error = this.element?.error;
    if (error) {
      this.onError(`Video playback failed (${error.code}): ${error.message || "Unable to decode the media."} H.265 playback requires browser and OS HEVC support.`);
    }
  };

  attach(element: HTMLVideoElement): void {
    this.element?.removeEventListener("error", this.handleMediaError);
    this.element = element;
    element.addEventListener("error", this.handleMediaError);
    this.player?.attachMediaElement(element);
  }

  load(source: string): void {
    if (!this.element) {
      throw new Error("Media player must be attached before loading a source.");
    }
    this.destroyPlayer();
    if (!this.mpegts.isSupported()) {
      throw new Error("This browser does not support FLV playback.");
    }
    this.player = this.mpegts.createPlayer({ type: "flv", url: source, isLive: false });
    this.player.on(this.mpegts.Events.ERROR, this.handlePlayerError);
    this.player.attachMediaElement(this.element);
    this.player.load();
  }

  play(): Promise<void> {
    return this.element?.play() ?? Promise.reject(new Error("Media player is not attached."));
  }

  pause(): void {
    this.element?.pause();
  }

  seekTo(seconds: number): void {
    if (this.element) this.element.currentTime = seconds;
  }

  private destroyPlayer(): void {
    this.player?.off(this.mpegts.Events.ERROR, this.handlePlayerError);
    this.player?.destroy();
    this.player = undefined;
  }

  destroy(): void {
    this.element?.removeEventListener("error", this.handleMediaError);
    this.destroyPlayer();
    this.element = undefined;
  }
}
