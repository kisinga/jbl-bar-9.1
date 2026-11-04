// Segmented level controls. The bar only steps to the next level, so a choice
// becomes the number of confirmed *_next steps that lead to it; the shell runs
// them through its action queue and capability mapping.
export function cycleSteps(value, target, count) {
  if (!Number.isInteger(value) || !Number.isInteger(target) || target < 0 || target >= count) return 0;
  return (target - value + count) % count;
}
// Marks the current value and, while steps are pending, the chosen one.
export function renderSegments(group, value, target, disabled) {
  if (!group) return;
  for (const button of group.querySelectorAll('button[data-value]')) {
    const v = Number(button.dataset.value), selected = v === value;
    button.classList.toggle('selected', selected);
    button.classList.toggle('target', target !== undefined && v === target && !selected);
    button.setAttribute('aria-checked', String(selected));
    if (disabled) button.disabled = true;
  }
}
// A target stays shown only while its steps are running or queued.
export function settleTarget(targets, name, value, action, pending) {
  if (targets[name] === value || !pending.includes(action)) delete targets[name];
  return targets[name];
}
