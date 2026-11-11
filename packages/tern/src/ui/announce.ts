// Controls that post through hidden inputs (Combobox, SelectMenu,
// DatePicker, Slider, …) don't fire input events as the user changes
// them. announce does: on the element it's on, after each change of its
// parameter, so a Form checks the field as the user changes it.

/** A Svelte action: use:announce={value} fires input when value changes. */
export function announce(node: HTMLElement, _value: unknown) {
	return {
		update() {
			// After Svelte has written the new value to the DOM, which it does
			// after calling update, so the form reads what the user chose.
			queueMicrotask(() => node.dispatchEvent(new Event('input', { bubbles: true })));
		}
	};
}
