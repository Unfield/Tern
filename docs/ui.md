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
| `Combobox` | the ARIA combobox with a list: typing filters (or calls `onsearch` for server results), arrow keys move, Enter picks, Escape closes and then clears. Leaving it with other text keeps the chosen option |
| `SelectMenu` | the ARIA select-only combobox, for options with descriptions and groups: type-ahead open or closed, Home, End and Page keys |
| `Command` | a filtered list of commands with groups, keywords and shortcuts. With `dialog` and `hotkey="k"`, a modal palette opened with ⌘K or Ctrl+K |
| `Calendar`, `DatePicker` | the ARIA date grid: arrow keys by day and week, Home and End for the week, Page keys by month and Shift+Page by year. Days are labelled in full and today is `aria-current`. Values are ISO dates (`2026-11-10`) that Go parses with `time.Parse(time.DateOnly, …)`; ranges take a start and an end |
| `Slider` | `role="slider"` thumbs with values, text and limits. A range has two thumbs that never cross. Arrow, Page, Home and End keys, and the pointer |
| `ToggleGroup`, `Toggle` | a single-choice group is a radio group with arrow keys; a multiple-choice group and `Toggle` are buttons with `aria-pressed` |
| `PasswordInput` | a native password input with a "Show password" button (`aria-pressed`) inside it. Password managers and `autocomplete` keep working |
| `InputOTP` | one real input with `autocomplete="one-time-code"`, drawn as boxes, so pasting and filling the code from a text message work. `oncomplete` runs once it's full |
| `InputGroup`, `ButtonGroup`, `Label` | an Input with text or buttons inside its border; joined buttons in a labelled `group`; a label for layouts without `Field` |
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
| `Sidebar` | an app shell: a labelled navigation with groups, badges and sub-links, current page marked, that collapses to icons and becomes a sheet on phones |
| `NavigationMenu` | site navigation with dropdown panels of links, built on the disclosure pattern (links don't belong in an ARIA menu). ArrowDown enters a panel, Escape returns |
| `Breadcrumb`, `Pagination` | labelled `nav`s with `aria-current="page"`. Long breadcrumbs fold their middle into a menu; pagination can render links (`href`) so it works without JavaScript |
| `DataTable`, `Table` | semantic tables with captions. `DataTable` sorts (`aria-sort`), searches, selects with checkboxes and pages, on the client or through `onquery` on the server, and announces what it shows |
| `LineChart`, `AreaChart`, `BarChart`, `DonutChart` | a figure with a heading, a legend for two or more series, a hover readout, arrow keys that read values aloud, and a "Show data" table. Colors are checked for color blindness in both modes |
| `Stat`, `Sparkline` | a headline number with a signed change (whose color knows whether up is good) and a trend |
| `Resizable` | two panes with a focusable splitter (`role="separator"`) moved by pointer or arrow keys |
| `Carousel` | `aria-roledescription` carousel and slides, previous and next buttons, slides out of view made inert, and a stop button when it plays by itself. It pauses on hover and focus |
| `Collapsible`, `ScrollArea`, `AspectRatio`, `Empty`, `Item` | a disclosure; a focusable, labelled scroll region; a fixed ratio box; an empty state; a list row whose whole area is its link |
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

### Charts

```svelte
<LineChart
	title="Traffic"
	data={days}
	x="day"
	series={[{ key: 'pageviews', label: 'Pageviews' }, { key: 'visitors', label: 'Visitors' }]}
/>
```

- **Data:** rows of plain objects. `x` may hold dates (`Date` or ISO strings, which get a time axis), numbers or categories.
- **Colors:** series take `--tern-chart-1` to `--tern-chart-8` in order. The palette is validated for color vision deficiencies on Tern's surfaces in light and dark mode. A ninth series is a sign to fold the rest into "Other", as `DonutChart` does.
- **Reading values:** hover shows every series at that point. Focus the chart and use the arrow keys to hear them. "Show data" shows them all as a table.

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
