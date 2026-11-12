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
| `SearchBox`, `Highlight` | a `role="search"` GET form that works without JavaScript; with `search`, an ARIA combobox whose results are links: arrow keys move, Enter opens, Escape closes and then clears, and the number of results is announced. `Highlight` marks matches with `<mark>` ([Search](search.md)) |
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

`Form` checks the rules of a [`kit.FormAction`](guide.md#validating-forms) as the user types, and shows the server's errors after a post:

```svelte
<Form rules={forms.default} {form} bind:submitting>
	<Field label="Email" name="email" description="We'll only email you about your account.">
		<Input type="email" autocomplete="email" />
	</Field>
	<Checkbox name="terms">I accept the terms</Checkbox>
	<Button type="submit" loading={submitting}>Create account</Button>
</Form>
```

- **`name` on `Field`:** the control inside posts under it, the Field shows its error and knows whether it's required, and the control refills with what was posted after a failure without JavaScript.
- **When it checks:** a field once the user has changed and left it, then as they type; everything on submit, without posting while anything is wrong. The first problem gets focus and a status line says how many there are.
- **Server errors:** `kit.FieldError` and the rules' own messages land on their fields; `kit.FormError` shows above the form.
- **Without JavaScript:** the controls are ordinary form elements, so the browser posts and Go answers with the same errors.

Outside a `Form`, pass `error` to a `Field` yourself.

## Blocks

Whole screens, wired to Tern's plugins:

| Block | |
|---|---|
| `SignIn` | buttons for `plugins/oauth` providers, a "Sign in with a passkey" button for the `accounts` package (`passkey`), an email and password `Form` for your own action, "Keep me signed in", and the error `plugins/oauth` sends back in `?error=` |
| `SignUp` | name, email, password (its hint comes from the action's `min` rule), an optional confirmation and terms checkbox |
| `ForgotPassword` | asks for the email to send a reset link to, then says it's on its way |
| `ResetPassword` | a new password, and its confirmation, for the token in the email's link; or, for an expired link, a way to ask for another |
| `Devices` | the devices a user is signed in on, from `acc.Sessions`, with a sign-out button on each and one for all the others |
| `LinkedAccounts` | the providers connected to an account, from `acc.Identities`, with Disconnect, and Connect buttons for the others |
| `TwoStep` | two-step sign-in on an account page: a QR code and the secret while setting up, the recovery codes once, and turning it off |
| `TwoStepCode` | the code page after a password sign-in: six boxes that post when full, or a recovery code |
| `Passkeys` | a user's passkeys, from `acc.Passkeys`: add one (the browser asks for a fingerprint, face or screen lock), see when each was last used, remove one |
| `ActivityLog` | the audit log, from `acc.Activity` (a user's own) or `acc.TeamActivity` (with `showActor`): what happened, when, from which device and address, with failed sign-ins marked. `labels` words the app's own actions |
| `NewTeam` | the page that makes a team, for `acc.CreateTeam` |
| `TeamSwitcher` | a menu of the user's teams, from `acc.Teams`, that switches the current one, with a link to make a new team. It fits a `Sidebar`'s header |
| `TeamMembers` | a team's members, from `acc.Members`, with their roles, a role menu where the user's own role allows changing it, Remove, and Leave on the user's own row. The last owner isn't offered Leave |
| `TeamInvitations` | an invite form, by email and role, and the pending invitations with Cancel. Admins aren't offered the owner role |
| `TeamSettings` | renaming the team, and deleting it after typing its name, for its owners |
| `AcceptInvitation` | the page an invitation email links to: join, or sign up or in with the invited address first, or sign out of another account |
| `PricingTable` | the plans from `billing`, by month or year with the yearly saving, each with its price, trial, limits and features, and a button that posts to a checkout action, or links to sign up |
| `BillingSettings` | the plan that applies, when it renews, ends or its trial does, a failed payment, how much of each limit is used, and buttons for Stripe's portal and the pricing page |
| `Admin` | the `admin` package's screens, from `admin.Load`: a menu of resources, lists with search, filters, sorting and pages, and forms to create, edit and delete records ([Admin screens](admin.md)) |
| `ConsentScreen` | the approval page for `plugins/oidc`, showing who is signed in and what the app may do. It posts the decision back to the plugin, which checks the request again |

```svelte
<SignIn providers={data.providers} rules={forms.default} {form} error={data.error} signUpHref="/signup" />
```

For `ConsentScreen`, set `ConsentURL: "/consent"` in the `plugins/oidc` options and return the request from the page's load:

```go
func Load(ctx *kit.Ctx) (oidc.Consent, error) { return provider.ConsentRequest(ctx) }
```

`ConsentRequest` checks the client and its redirect address, sends signed-out users to sign in, and stops other sites from framing the page.

## Owning a component

Most apps import the components and theme them with CSS variables. To change one beyond that, copy it into the project:

```
$ go tool tern ui add DataTable
  ✓ src/lib/ui/Checkbox.svelte
  ✓ src/lib/ui/DataTable.svelte
  ...
```

- **What it copies:** the component and the components and helpers it uses, from the installed `@ternkit/tern`, into `src/lib/ui` (`-dir` changes it). Imports of Tern's runtime point at `@ternkit/tern`.
- **Importing:** import from `$lib/ui/DataTable.svelte` instead. The copy is yours: updating Tern doesn't change it.
- **Running it again:** files already there are kept; `-force` replaces them. `tern ui list` lists the components.

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
