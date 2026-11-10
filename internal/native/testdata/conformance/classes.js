// Private methods exist before field initializers run.
class A {
	#value = 3;
	#persisted = this.#make();
	#make(v = this.#value) { return v * 2; }
	static #count = 0;
	static next() { return ++A.#count; }
	get p() { return this.#persisted; }
	get #secret() { return 'hidden'; }
	reveal() { return this.#secret; }
	has(o) { return #value in o; }
}
class B extends A {
	extra = super.p + 1;
	constructor(x) { super(); this.x = x; }
	toString() { return `B(${this.x})`; }
}
const b = new B(5);
globalThis.out = [new A().p, A.next(), A.next(), b.p, b.extra, String(b), b.reveal(), b.has(b), b.has({}), b instanceof A, Object.keys(b)];
