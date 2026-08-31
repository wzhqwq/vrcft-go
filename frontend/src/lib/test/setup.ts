import '@testing-library/jest-dom/vitest';
import {cleanup} from '@testing-library/svelte';
import {afterEach} from 'vitest';

class TestResizeObserver implements ResizeObserver {
  constructor(_callback: ResizeObserverCallback) {}

  disconnect() {}
  observe(_target: Element, _options?: ResizeObserverOptions) {}
  unobserve(_target: Element) {}
}

if (!window.ResizeObserver) {
  window.ResizeObserver = TestResizeObserver;
}

afterEach(() => {
  cleanup();
});
