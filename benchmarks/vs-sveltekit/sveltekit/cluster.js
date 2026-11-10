// Runs the built app on every core, as a production SvelteKit deployment
// would with Node's cluster module (or PM2).
import cluster from 'node:cluster';
import { availableParallelism } from 'node:os';

const n = Number(process.env.WORKERS) || availableParallelism();
if (cluster.isPrimary) {
	for (let i = 0; i < n; i++) cluster.fork();
} else {
	await import('./build/index.js');
}
