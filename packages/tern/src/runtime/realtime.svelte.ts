import { onDestroy } from 'svelte';

export type ConnectionStatus = 'connecting' | 'open' | 'closed';

/** Closes c when the component that created it is destroyed, if any. */
function closeWithComponent(c: { close(): void }) {
	try {
		onDestroy(() => c.close());
	} catch {
		// Created outside a component: the caller closes it.
	}
}

const browser = typeof window !== 'undefined';

function parse(data: string): unknown {
	try {
		return JSON.parse(data);
	} catch {
		return data;
	}
}

export interface StreamOptions<T> {
	/** Event names to listen for. The default is the unnamed "message" event. */
	events?: string[];
	/** The value of current before the first event, such as data from a load. */
	initial?: T;
	/** Called for each event, with its decoded data and name. */
	onMessage?: (data: T, event: string) => void;
	/** Send cookies to another origin. */
	withCredentials?: boolean;
}

/** A server-sent event stream; see stream(). */
export class EventStreamState<T> {
	/** The data of the latest event, decoded from JSON when it is JSON. */
	current = $state<T | undefined>(undefined);
	/** The connection's status. The browser reconnects by itself after errors. */
	status = $state<ConnectionStatus>('connecting');
	#source: EventSource | null = null;

	constructor(url: string, options: StreamOptions<T> = {}) {
		this.current = options.initial;
		if (!browser) {
			this.status = 'closed';
			return;
		}
		const source = new EventSource(url, { withCredentials: options.withCredentials });
		this.#source = source;
		const handle = (e: MessageEvent<string>) => {
			const data = parse(e.data) as T;
			this.current = data;
			options.onMessage?.(data, e.type);
		};
		for (const name of options.events?.length ? options.events : ['message']) source.addEventListener(name, handle as EventListener);
		source.onopen = () => (this.status = 'open');
		source.onerror = () => (this.status = source.readyState === EventSource.CLOSED ? 'closed' : 'connecting');
		closeWithComponent(this);
	}

	/** Closes the stream. Streams created in a component close with it. */
	close() {
		this.#source?.close();
		this.#source = null;
		this.status = 'closed';
	}
}

/**
 * Subscribes to server-sent events from an endpoint that uses
 * ctx.EventStream or realtime.Stream:
 *
 * ```svelte
 * <script lang="ts">
 *   import { stream } from '@ternkit/tern';
 *   let { data } = $props();
 *   const monitors = stream<Monitor[]>('/api/monitors/events', { events: ['monitors'], initial: data.monitors });
 * </script>
 *
 * {#each monitors.current ?? [] as m}…{/each}
 * ```
 *
 * During server rendering it does not connect and current is the initial
 * value. Created in a component, it closes when the component is destroyed.
 */
export function stream<T = unknown>(url: string, options?: StreamOptions<T>): EventStreamState<T> {
	return new EventStreamState<T>(url, options);
}

export interface SocketOptions<In> {
	/** Called for each message, decoded from JSON when it is JSON. */
	onMessage?: (message: In) => void;
	/** WebSocket subprotocols. */
	protocols?: string[];
	/** Reconnect with backoff after the connection drops. The default is true. */
	reconnect?: boolean;
}

/** A WebSocket connection; see socket(). */
export class SocketState<In = unknown, Out = unknown> {
	/** The latest message received. */
	last = $state<In | undefined>(undefined);
	/** The connection's status. */
	status = $state<ConnectionStatus>('connecting');
	#ws: WebSocket | null = null;
	#queue: string[] = [];
	#closed = false;
	#attempt = 0;
	#timer: ReturnType<typeof setTimeout> | undefined;
	#url: string;
	#options: SocketOptions<In>;

	constructor(url: string, options: SocketOptions<In> = {}) {
		this.#url = url;
		this.#options = options;
		if (!browser) {
			this.status = 'closed';
			return;
		}
		this.#connect();
		closeWithComponent(this);
	}

	#connect() {
		const u = new URL(this.#url, location.href);
		if (u.protocol === 'http:') u.protocol = 'ws:';
		else if (u.protocol === 'https:') u.protocol = 'wss:';
		const ws = new WebSocket(u, this.#options.protocols);
		this.#ws = ws;
		this.status = 'connecting';
		ws.onopen = () => {
			this.#attempt = 0;
			this.status = 'open';
			for (const m of this.#queue.splice(0)) ws.send(m);
		};
		ws.onmessage = (e) => {
			const msg = (typeof e.data === 'string' ? parse(e.data) : e.data) as In;
			this.last = msg;
			this.#options.onMessage?.(msg);
		};
		ws.onclose = (e) => {
			this.#ws = null;
			// 1000 is a normal close by the server: don't come back.
			if (this.#closed || this.#options.reconnect === false || e.code === 1000) {
				this.status = 'closed';
				return;
			}
			this.status = 'connecting';
			const delay = Math.min(10_000, 500 * 2 ** this.#attempt++) * (0.75 + Math.random() / 2);
			this.#timer = setTimeout(() => this.#connect(), delay);
		};
	}

	/** Sends a message as JSON (strings as is). Messages sent while connecting are queued. */
	send(message: Out) {
		const data = typeof message === 'string' ? message : JSON.stringify(message);
		if (this.#ws?.readyState === WebSocket.OPEN) this.#ws.send(data);
		else if (!this.#closed) this.#queue.push(data);
	}

	/** Closes the connection. Sockets created in a component close with it. */
	close() {
		this.#closed = true;
		clearTimeout(this.#timer);
		this.#ws?.close(1000);
		this.#ws = null;
		this.status = 'closed';
	}
}

/**
 * Opens a WebSocket to an endpoint that uses ctx.WebSocket, with JSON
 * messages and automatic reconnection:
 *
 * ```svelte
 * <script lang="ts">
 *   import { socket } from '@ternkit/tern';
 *   const chat = socket<ChatMessage, { text: string }>('/api/chat', { onMessage: (m) => messages.push(m) });
 * </script>
 *
 * <button onclick={() => chat.send({ text })}>Send</button>
 * ```
 *
 * It does not connect during server rendering. Created in a component, it
 * closes when the component is destroyed.
 */
export function socket<In = unknown, Out = unknown>(url: string, options?: SocketOptions<In>): SocketState<In, Out> {
	return new SocketState<In, Out>(url, options);
}
