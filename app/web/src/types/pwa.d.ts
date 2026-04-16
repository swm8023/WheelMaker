declare global {
  interface Window {
    __WHEELMAKER_PWA__?: import('../pwa').PWAFoundation;
  }
}

export {};