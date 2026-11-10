/** An entry in a Menu, ContextMenu or Menubar. */
export type MenuItem = {
	/** The text; also what typing a letter jumps to. */
	label?: string;
	onselect?: () => void;
	/** Makes the item a link. */
	href?: string;
	danger?: boolean;
	disabled?: boolean;
	/** A keyboard shortcut to show, such as "⌘K". It's only shown: bind the key yourself. */
	shortcut?: string;
	/** For type "checkbox" or "radio": whether it's checked. Flip it in onselect. */
	checked?: boolean;
	type?: 'checkbox' | 'radio';
	/** Items of a submenu, opened with ArrowRight, Enter or by hovering. */
	items?: MenuItem[];
	/** Draws a line instead of an item. */
	separator?: boolean;
	/** Starts a labelled group of the items after it. */
	heading?: string;
};

export type MenuClose = { all: boolean; returnFocus: boolean };
