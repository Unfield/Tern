<script lang="ts">
	import { Accordion, Dialog, Select, Tabs, Tooltip } from 'bits-ui';
	let value = $state('b');
</script>

<Tooltip.Provider>
	<Accordion.Root type="single" value="one">
		<Accordion.Item value="one">
			<Accordion.Header><Accordion.Trigger>First</Accordion.Trigger></Accordion.Header>
			<Accordion.Content>Accordion content</Accordion.Content>
		</Accordion.Item>
	</Accordion.Root>
	<Tabs.Root value="a">
		<Tabs.List><Tabs.Trigger value="a">Tab A</Tabs.Trigger></Tabs.List>
		<Tabs.Content value="a">Tab panel</Tabs.Content>
	</Tabs.Root>
	<Select.Root type="single" bind:value items={[{ value: 'a', label: 'A' }, { value: 'b', label: 'B' }]}>
		<Select.Trigger>Selected {value}</Select.Trigger>
	</Select.Root>
	<Dialog.Root><Dialog.Trigger>Open dialog</Dialog.Trigger></Dialog.Root>
	<Tooltip.Root><Tooltip.Trigger>Hover me</Tooltip.Trigger></Tooltip.Root>
</Tooltip.Provider>
