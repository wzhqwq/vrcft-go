import {fireEvent, render, screen} from '@testing-library/svelte';
import {describe, expect, it} from 'vitest';

import {NumberField, SelectField, SwitchField, TextField} from './index.js';

describe('field controls', () => {
  it('connects text-field label, description, error, and required state', () => {
    render(TextField, {
      props: {label: '地址', description: '目标地址', error: '必填', required: true, value: ''},
    });

    const input = screen.getByRole('textbox', {name: '地址'});
    expect(input).toHaveAccessibleDescription('目标地址 必填');
    expect(input).toBeRequired();
    expect(input).toHaveAttribute('aria-invalid', 'true');
  });

  it('keeps number-field disabled state on its labelled input', () => {
    render(NumberField, {props: {label: '端口', value: 9000, disabled: true}});

    expect(screen.getByRole('spinbutton', {name: '端口'})).toBeDisabled();
  });

  it('announces select errors through its labelled control', () => {
    render(SelectField, {
      props: {
        label: '模式',
        description: '选择追踪模式',
        error: '请选择模式',
        required: true,
        value: 'auto',
        options: [{value: 'auto', label: '自动'}, {value: 'manual', label: '手动'}],
      },
    });

    const control = screen.getByRole('button', {name: '模式'});
    expect(control).toHaveAccessibleDescription('选择追踪模式 请选择模式');
    expect(control).toHaveAttribute('aria-invalid', 'true');
    expect(control).toHaveAttribute('aria-required', 'true');
  });

  it('updates the checked switch state when activated', async () => {
    render(SwitchField, {props: {label: '启用 OSC', checked: false, required: true}});

    const control = screen.getByRole('switch', {name: '启用 OSC'});
    expect(control).not.toBeChecked();
    expect(control).toHaveAttribute('aria-required', 'true');
    await fireEvent.click(control);
    expect(control).toBeChecked();
  });
});
