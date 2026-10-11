import { afterEach, describe, expect, it, vi } from 'vitest';
import { mount } from '@vue/test-utils';
import { computed } from 'vue';
import zh from '../../../i18n/locales/zh.json';
import SetupConnectionFeedback from '~/components/setup/ConnectionFeedback.vue';
import { describeSetupConnectionError } from '~/utils/setup-connection-test';

afterEach(() => vi.unstubAllGlobals());

describe('setup connection errors', () => {
  it.each([
    ['NOAUTH Authentication required.', 'redisAuthRequired'],
    ['WRONGPASS invalid username-password pair or user is disabled.', 'redisAuthInvalid'],
    ['NOPERM this user has no permissions to run the ping command', 'redisPermissionDenied'],
  ])('explains Redis protocol error %s', (error, key) => {
    expect(describeSetupConnectionError({ data: { error, request_id: 'request-1' } }, 'cache'))
      .toMatchObject({ messageKey: `setup.connection.${key}`, diagnosticId: 'request-1' });
  });
  it('reads native ofetch errors and the response trace header', () => {
    expect(describeSetupConnectionError({ response: {
      _data: { error: { message: 'NOAUTH Authentication required.' } },
      headers: new Headers({ 'x-trace-id': 'trace-1' }),
    } }, 'cache')).toMatchObject({ messageKey: 'setup.connection.redisAuthRequired', diagnosticId: 'trace-1' });
  });
  it('preserves a database error without classifying it as Redis authentication', () => {
    expect(describeSetupConnectionError({ data: { error: 'database connection refused' } }, 'database'))
      .toMatchObject({ messageKey: undefined, detail: 'database connection refused' });
  });
  it('does not show an object as a diagnostic message', () => {
    expect(describeSetupConnectionError({ data: { error: {} } }, 'cache').detail).toBeUndefined();
  });
});

describe('setup connection feedback', () => {
  const create = (props: Record<string, unknown>) => {
    vi.stubGlobal('computed', computed);
    vi.stubGlobal('useI18n', () => ({ t: (key: string, params: Record<string, string> = {}) => {
      let value: any = zh;
      for (const segment of key.split('.')) value = value[segment];
      return Object.entries(params).reduce((s, [name, text]) => s.replace(`{${name}}`, text), String(value));
    } }));
    return mount(SetupConnectionFeedback, {
      props: { kind: 'cache', testing: false, ok: false, message: '', ...props },
      global: { stubs: { UAlert: {
        props: ['color', 'title', 'description', 'role'],
        template: '<div :role="role" :data-color="color"><strong>{{ title }}</strong><p>{{ description }}</p></div>',
      } } },
    });
  };
  it('shows a persistent error alert and request reference', () => {
    const message = zh.setup.connection.redisAuthRequired;
    const wrapper = create({ message, diagnosticId: 'request-1' });
    expect(wrapper.get('[role="alert"]').attributes('data-color')).toBe('error');
    expect(wrapper.text()).toContain(message);
    expect(wrapper.text()).toContain('request-1');
    wrapper.unmount();
  });
  it('retains the previous result while indicating that a retry is running', async () => {
    const message = zh.setup.connection.redisAuthRequired;
    const wrapper = create({ message });
    await wrapper.setProps({ testing: true });
    expect(wrapper.get('[role="status"]').attributes('data-color')).toBe('info');
    expect(wrapper.text()).toContain(zh.setup.connection.cache.testing);
    expect(wrapper.text()).toContain(message);
    wrapper.unmount();
  });
  it('distinguishes a successful result from an idle hint', async () => {
    const wrapper = create({});
    expect(wrapper.find('[role="alert"]').exists()).toBe(false);
    await wrapper.setProps({ ok: true, message: zh.setup.connection.cache.success });
    expect(wrapper.get('[role="status"]').attributes('data-color')).toBe('success');
    wrapper.unmount();
  });
});
