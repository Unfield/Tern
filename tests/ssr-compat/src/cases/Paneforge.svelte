<script lang="ts">
	import { Pane, PaneGroup, PaneResizer } from 'paneforge';
</script>

<PaneGroup direction="horizontal">
	<Pane defaultSize={30}>Left pane</Pane>
	<PaneResizer />
	<Pane defaultSize={70}>Right pane</Pane>
</PaneGroup>
