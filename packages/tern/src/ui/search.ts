/** A piece of a search snippet: text that matched, or text around it. The search package's Segment. */
export type Segment = { text: string; match?: boolean };

/** A result in a SearchBox. */
export type SearchItem = {
	/** Where choosing it goes. */
	href: string;
	/** Its title, plain or with the matches marked. */
	title: string | Segment[];
	/** Text around what matched, such as a snippet of its body. */
	snippet?: Segment[];
	/** A short line beside the title, such as its section. */
	meta?: string;
};
