export interface MediaPlayer {
  attach(element: HTMLVideoElement): void;
  load(source: string): void;
  play(): Promise<void>;
  pause(): void;
  seekTo(seconds: number): void;
  destroy(): void;
}

type FlvModule = typeof import("flv.js");

export class FlvMediaPlayer implements MediaPlayer {
  private readonly flv: FlvModule;
  private player: ReturnType<FlvModule["createPlayer"]> | undefined;
  private element: HTMLVideoElement | undefined;

  constructor(flv: FlvModule) {
    this.flv = flv;
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
    if (!this.flv.isSupported()) {
      throw new Error("This browser does not support FLV playback.");
    }
    this.player = this.flv.createPlayer({ type: "flv", url: source });
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
