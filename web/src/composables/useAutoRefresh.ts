import { ref, onBeforeUnmount } from 'vue';

const REFRESH_INTERVAL = 30_000;

export const useAutoRefresh = (callback: () => void) => {
  const enabled = ref(true);
  let timer: ReturnType<typeof setInterval> | null = null;

  const start = (): void => {
    stop();
    timer = setInterval(callback, REFRESH_INTERVAL);
  };

  const stop = (): void => {
    if (timer !== null) {
      clearInterval(timer);
      timer = null;
    }
  };

  const toggle = (on: boolean): void => {
    enabled.value = on;
    if (on) {
      start();
    } else {
      stop();
    }
  };

  start();
  onBeforeUnmount(stop);

  return { enabled, toggle };
};
