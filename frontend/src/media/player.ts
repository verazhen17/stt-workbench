import type mpegts from "mpegts.js";

export interface MediaPlayer {
  attach(element: HTMLVideoElement): void;
  load(source: string): void;
  play(): Promise<void>;
  pause(): void;
  seekTo(seconds: number): void;
  destroy(): void;
}

export type MpegtsModule = typeof mpegts;

export class FlvMediaPlayer implements MediaPlayer {
  private readonly mpegts: MpegtsModule;
  private player: mpegts.Player | undefined;
  private element: HTMLVideoElement | undefined;
  private readonly onError?: (error: Error) => void;

  constructor(mpegts: MpegtsModule, onError?: (error: Error) => void) {
    this.mpegts = mpegts;
    this.onError = onError;
  }

  attach(element: HTMLVideoElement): void {
    this.element = element;
    this.player?.attachMediaElement(element);
  }

  load(source: string): void {
    if (!this.element) {
      throw new Error("Media player must be attached before loading a source.");
    }
    this.player?.destroy();
    this.player = undefined;
    if (!this.mpegts.isSupported()) {
      throw new Error("This browser does not support MSE / FLV playback.");
    }
    this.player = this.mpegts.createPlayer(
      {
        type: "flv",
        url: source,
        isLive: false,
        hasAudio: true,
        hasVideo: true,
      },
      {
        enableWorker: true,
        lazyLoad: false,
        seekType: "range",
      }
    );
    if (this.onError) {
      const errorHandler = this.onError;
      this.player.on(this.mpegts.Events.ERROR, (errorType: string, errorDetail: string) => {
        errorHandler(new Error(`Playback error: ${errorType} (${errorDetail})`));
      });
    }
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

  destroy(): void {
    this.player?.destroy();
    this.player = undefined;
    this.element = undefined;
  }
}
