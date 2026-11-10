# Tern UI

`@ternkit/tern/ui` is a set of accessible Svelte 5 components with Tern's own look: calm surfaces, crisp borders, a deep sea-teal accent, and a clear focus ring. They follow the WAI-ARIA patterns, work with the keyboard, and are audited with [axe](https://github.com/dequelabs/axe-core) in CI, in light and dark mode. They need no CSS framework.

```svelte
<!-- src/routes/+layout.svelte -->
<script lang="ts">
	import '@ternkit/tern/ui/theme.css';
	import { Toaster } from '@ternkit/tern/ui';

	let { children } = $props();
</script>

{@render children()}
<Toaster />
```

```svelte
<script lang="ts">
	import { Button, Field, Input, toast } from '@ternkit/tern/ui';
</script>

<Field label="Project name" description="Shown on your status page.">
	<Input name="name" />
</Field>
<Button onclick={() => toast.success('Saved')}>Save</Button>
```

See [examples/ui](../examples/ui) for every component, and a form validated by a Go action.

## Components

| Component | Accessibility |
|---|---|
| `Button` | a native `<button>`, or `<a>` with `href`. `loading` sets `aria-busy` and blocks clicks but keeps focus |
| `Field` with `Input`, `Textarea` or `Select` | the label, description and error are wired to the control with `for`, `aria-describedby` and `aria-invalid`. The error is announced |
| `Checkbox`, `RadioGroup`, `Select` | native inputs, styled: arrow keys and screen readers behave as users expect. `RadioGroup` is a `fieldset` with a `legend` |
| `Switch` | `role="switch"` with `aria-checked`. It posts `on`/`off` with `name` |
| `Dialog` | a native modal `<dialog>`: the page behind is inert, Escape closes it, focus returns to the opener, and Tab cycles inside. It is labelled by its title and described by its description |
| `Tabs` | the ARIA tabs pattern: arrow keys, Home and End, roving tabindex, and disabled tabs skipped |
| `Menu` | the ARIA menu button pattern: Enter, Space and the arrow keys open it, type-ahead jumps, Escape returns focus, and Tab and clicks outside close it. Items can have submenus, checkboxes, radio items, shortcuts and labelled groups |
| `Menubar` | the ARIA menubar pattern: arrow keys move along the bar and between open menus, with a roving tabindex |
| `ContextMenu` | opens on right-click, a long press, Shift+F10 or the menu key, at the pointer or the focused element, and returns focus |
| `Popover` | a non-modal `role="dialog"` labelled by its title, opened by a trigger with `aria-expanded`. Focus moves in; Escape, clicks outside and tabbing away close it |
| `HoverCard` | a preview on hover or focus, linked as the trigger's description; Escape hides it |
| `AlertDialog` | `role="alertdialog"` for confirmations. Focus starts on Cancel, so Enter doesn't confirm by accident. The confirm button waits for an async `onconfirm` |
| `Sheet`, `Drawer` | modal dialogs against an edge. A `Drawer` is a bottom sheet that closes when dragged down |
| `Tooltip` | `role="tooltip"`, linked to its trigger with `aria-describedby`. It shows on hover and focus, Escape hides it, and it can sit on any `side` |
| `Accordion` | buttons with `aria-expanded` inside headings of the level you choose, and labelled regions |
| `Toaster` and `toast()` | a labelled notifications region. Errors are announced at once, the rest politely. Timers pause on hover and focus |
| `Alert` | with `live`, `role="alert"` (danger, warning) or `role="status"` |
| `Progress` | `role="progressbar"` with values; with no value, it is indeterminate |
| `Spinner` | `role="status"` with a hidden label |
| `Card`, `Badge`, `Avatar`, `Skeleton`, `Separator`, `Kbd` | semantic elements. `Card` takes a heading `level` and `as` (section, article). `Avatar` falls back to initials labelled with the name |

Tables use `<table class="tern-table">`, and `.tern-sr-only` hides text visually while keeping it for screen readers.

### Overlays

Popovers, menus, tooltips and hover cards open in the browser's top layer, through the Popover API. A scrolling container or a `z-index` never clips them, and they flip and shift to stay on screen. Choose where they go with `side` (`top`, `bottom`, `left`, `right`) and `align` (`start`, `center`, `end`).

Triggers are your own elements. The component passes the attributes they need through a snippet, so spread them on:

```svelte
<Popover title="Dimensions">
	{#snippet trigger(props)}<Button variant="secondary" {...props}>Set width</Button>{/snippet}
	<Field label="Width"><Input bind:value={width} /></Field>
</Popover>
```

Menus take items:

```ts
const items: MenuItem[] = [
	{ label: 'New file', shortcut: '⌘N', onselect: create },
	{ label: 'Open recent', items: recent.map((f) => ({ label: f.name, onselect: () => open(f) })) },
	{ separator: true },
	{ heading: 'View' },
	{ label: 'Word wrap', type: 'checkbox', checked: wrap, onselect: () => (wrap = !wrap) },
	{ label: 'Delete', danger: true, onselect: remove }
];
```

## Forms

The controls are ordinary form elements with `name`s, so they post with forms and work without JavaScript. This form shows errors from a [form action](guide.md#form-actions):

```svelte
<form method="POST" use:enhance>
	<Field label="Email" error={form?.errors?.email} required>
		<Input name="email" type="email" value={form?.values?.email ?? ''} />
	</Field>
	<Button type="submit">Sign up</Button>
</form>
```

## Theming

Every color, radius and shadow is a CSS custom property, so a theme is a few overrides:

```css
:root {
	--tern-accent: oklch(0.55 0.2 290);
	--tern-accent-hover: oklch(0.49 0.2 290);
	--tern-accent-soft: oklch(0.95 0.04 290);
	--tern-ring: oklch(0.62 0.18 290);
	--tern-radius: 14px;
}
```

- **Dark mode:** it follows the system. Set `data-theme="dark"` or `"light"` on `<html>` to choose.
- **Part of a page:** overrides on any element apply to the components inside it.
- **Motion:** it respects `prefers-reduced-motion`.
- **The full list:** see [theme.css](../packages/tern/src/ui/theme.css).
