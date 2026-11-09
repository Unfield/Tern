/** A resolved or failed deferred value, as streamed by the server. */
export interface Chunk {
    id: number;
    data?: unknown;
    error?: {
        message: string;
    };
}
/**
 * Connects the deferred placeholders of one response to the chunks that
 * resolve them. IDs are only unique within a response.
 */
export declare class DeferredScope {
    #private;
    promise(id: number): Promise<unknown>;
    settle(chunk: Chunk): void;
    /** Rejects every value still pending, e.g. when the stream ends early. */
    close(reason: string): void;
}
/** Replaces deferred placeholders in data with promises from scope. */
export declare function revive<T>(value: T, promise: (id: number) => Promise<unknown>): T;
/**
 * Reads a response that holds one JSON payload line, optionally followed
 * by one line per resolved deferred value. Resolves once the payload is
 * read; deferred values keep streaming in the background.
 */
export declare function readStreamed<P extends {
    nodes?: unknown;
}>(res: Response): Promise<P>;
