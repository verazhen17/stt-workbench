declare module "flv.js" {
  export interface MediaDataSource {
    type: "flv";
    url: string;
  }

  export interface Player {
    attachMediaElement(element: HTMLVideoElement): void;
    load(): void;
    destroy(): void;
  }

  export function isSupported(): boolean;
  export function createPlayer(source: MediaDataSource): Player;
}
