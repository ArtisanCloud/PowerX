import { defineComponent, ref } from 'vue';
import { mount } from '@vue/test-utils';
import { describe, expect, it } from 'vitest';
import BackupSectionTabs from '~/components/ops/backup/BackupSectionTabs.vue';

describe('vertical backup sections', () => {
  it('shows one panel and preserves its draft when switching by mouse or keyboard', async () => {
    const Host = defineComponent({
      components: { BackupSectionTabs },
      setup: () => ({ current: ref('files'), draft: ref(''), items: [{value:'files',label:'Files'}, {value:'policies',label:'Policies'}] }),
      template: '<BackupSectionTabs v-model="current" :items="items" label="Backup tasks"><template #files><input v-model="draft" aria-label="draft"/></template><template #policies><p>Policy settings</p></template></BackupSectionTabs>',
    });
    const w = mount(Host, {attachTo:document.body,global:{stubs:{UIcon:true}}});
    expect(w.get('[role="tablist"]').attributes('aria-orientation')).toBe('vertical');
    expect(w.get('[data-section="files"]').isVisible()).toBe(true);
    expect(w.get('[data-section="policies"]').isVisible()).toBe(false);
    await w.get('input').setValue('keep this draft');
    await w.findAll('[role="tab"]')[1]!.trigger('click');
    expect(w.get('[data-section="files"]').isVisible()).toBe(false);
    expect(w.get('[data-section="policies"]').isVisible()).toBe(true);
    await w.findAll('[role="tab"]')[1]!.trigger('keydown', {key:'ArrowUp'});
    expect(w.get('[data-section="files"]').isVisible()).toBe(true);
    expect(w.get('input').element.value).toBe('keep this draft');
    expect(w.findAll('[role="tab"]')[0]!.attributes('aria-selected')).toBe('true');
    await w.findAll('[role="tab"]')[0]!.trigger('keydown', {key:'End'});
    expect(w.get('[data-section="policies"]').isVisible()).toBe(true);
    await w.get('[data-section-select]').setValue('files');
    expect(w.get('[data-section="files"]').isVisible()).toBe(true);
    expect(w.get('input').element.value).toBe('keep this draft');
    w.unmount();
  });
});
