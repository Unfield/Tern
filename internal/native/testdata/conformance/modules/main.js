// Modules evaluate after their imports, once; import() loads a module
// nothing imported yet, later, in a job.
import { log } from './log.js';
import { a } from './a.js';
import { b } from './b.js';
log.push('main ' + a + b);
// Chained, so the order doesn't depend on how a loader schedules: bundles
// load in a job, Node's loader takes more ticks for an unloaded module.
import('./lazy.js')
	.then((m) => (log.push('lazy loaded ' + m.value), import('./a.js')))
	.then((m) => log.push('a again ' + m.a));
log.push('main end');
globalThis.out = log;
