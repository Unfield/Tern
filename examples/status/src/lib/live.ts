/** A check as streamed by /api/events (see monitor.Event in Go). */
export type Event = {
	monitor: { id: number; name: string; group: string; status: string; lastCheck: number | null; lastLatency: number | null };
	beat: { monitorId: number; at: number; status: string; latencyMs: number; message: string };
	target?: string;
};
