import type { Snippet } from 'svelte';

/** A column of a Table or DataTable. */
export type Column<R> = {
	/** The field shown, or an id when cell draws it. */
	key: keyof R & string;
	header: string;
	/** end for numbers. */
	align?: 'start' | 'end' | 'center';
	/** Draws the cell instead of the value. */
	cell?: Snippet<[R]>;
	/** For DataTable: whether the column sorts. */
	sortable?: boolean;
	/** For DataTable: the value it sorts and filters by, if not the field. */
	value?: (row: R) => string | number | Date | null | undefined;
	width?: string;
};

/** What a DataTable shows: passed to onquery when the server sorts, searches and pages. */
export type DataQuery = { page: number; pageSize: number; sort: { key: string; dir: 'asc' | 'desc' } | null; search: string };
