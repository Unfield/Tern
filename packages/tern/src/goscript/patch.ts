// Applying the patches live components send; see internal/jsondiff.

type Key = string | number;

/** One patch operation from the server. */
export type PatchOp = ['s', Key[], unknown] | ['d', Key[]] | ['a', Key[], number, number, unknown[]];

/** Applies a patch to root and returns the new root. */
export function applyPatch(root: unknown, ops: PatchOp[]): unknown {
	for (const op of ops) {
		const path = op[1];
		if (path.length === 0) {
			if (op[0] === 's') root = op[2];
			else if (op[0] === 'a') (root as unknown[]).splice(op[2], op[3], ...op[4]);
			continue;
		}
		let parent = root as Record<Key, unknown>;
		for (const k of path.slice(0, -1)) parent = parent[k] as Record<Key, unknown>;
		const last = path[path.length - 1]!;
		if (op[0] === 's') parent[last] = op[2];
		else if (op[0] === 'd') delete parent[last];
		else (parent[last] as unknown[]).splice(op[2], op[3], ...op[4]);
	}
	return root;
}
