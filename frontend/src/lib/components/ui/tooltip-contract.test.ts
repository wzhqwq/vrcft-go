import type {ComponentProps} from 'svelte';
import {expect, test} from 'vitest';

import Tooltip from './Tooltip.svelte';

type TooltipProps = ComponentProps<typeof Tooltip>;

test('Tooltip props require a labelled default trigger or a custom trigger', () => {
  const labelled: TooltipProps = {content: '帮助内容', triggerLabel: '帮助'};
  // @ts-expect-error Tooltip must not expose an unlabeled default button.
  const unlabeled: TooltipProps = {content: '帮助内容'};

  expect(labelled.triggerLabel).toBe('帮助');
  expect(unlabeled.content).toBe('帮助内容');
});
