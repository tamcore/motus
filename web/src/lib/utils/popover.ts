export function placeBelowInvoker(event: ToggleEvent): void {
  const popover = event.currentTarget as HTMLElement;
  if (event.newState !== "open" || CSS.supports("position-anchor: --a")) return;
  const invoker = document.querySelector(`[popovertarget="${popover.id}"]`);
  if (!invoker) return;
  const rect = invoker.getBoundingClientRect();
  popover.style.top = `${rect.bottom}px`;
  popover.style.left = `${Math.max(0, rect.right - popover.offsetWidth)}px`;
}
