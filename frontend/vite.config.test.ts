import {describe, expect, it} from 'vitest';

import {isComponentWorkbenchEnabled} from './vite.config.js';

describe('component workbench compile flag', () => {
  it('requires the explicit development flag and defaults production to disabled', () => {
    expect(isComponentWorkbenchEnabled('development', 'true')).toBe(true);
    expect(isComponentWorkbenchEnabled('development', undefined)).toBe(false);
    expect(isComponentWorkbenchEnabled('production', 'true')).toBe(false);
  });
});
