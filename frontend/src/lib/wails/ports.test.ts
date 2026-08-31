import {describe, expect, it, vi} from 'vitest';

import {createMockPorts} from './testing';

describe('createMockPorts', () => {
  it('keeps runtime and plugins listeners independent', () => {
    const ports = createMockPorts();
    const runtime = vi.fn();
    const plugins = vi.fn();

    const stop = ports.runtime.onChanged(runtime);
    ports.plugins.onChanged(plugins);

    stop();
    ports.events.runtime({revision: 2});
    ports.events.plugins({revision: 2});

    expect(runtime).not.toHaveBeenCalled();
    expect(plugins).toHaveBeenCalledOnce();
  });
});
