import '@testing-library/jest-dom/vitest';
import {cleanup} from '@testing-library/svelte';
import {afterEach} from 'vitest';

// Existing UI fixtures assert the Chinese copy. Locale resolution itself has
// dedicated tests for English and unsupported system languages.
Object.defineProperty(navigator, 'language', {configurable: true, value: 'zh-CN'});

class TestResizeObserver implements ResizeObserver {
  constructor(_callback: ResizeObserverCallback) {}

  disconnect() {}
  observe(_target: Element, _options?: ResizeObserverOptions) {}
  unobserve(_target: Element) {}
}

if (!window.ResizeObserver) {
  window.ResizeObserver = TestResizeObserver;
}

if (!HTMLElement.prototype.hasPointerCapture) {
  HTMLElement.prototype.hasPointerCapture = () => false;
}

if (!HTMLElement.prototype.releasePointerCapture) {
  HTMLElement.prototype.releasePointerCapture = () => {};
}

afterEach(() => {
  cleanup();
});
