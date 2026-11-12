// Checks the extension in a real VS Code: routes, page ⇄ load, and
// diagnostics from tern check.
const assert = require('node:assert');
const path = require('node:path');
const fs = require('node:fs');
const vscode = require('vscode');

const wait = (ms) => new Promise((r) => setTimeout(r, ms));
async function until(what, fn, ms = 60000) {
	const end = Date.now() + ms;
	for (;;) {
		const v = await fn();
		if (v) return v;
		if (Date.now() > end) throw new Error('timed out waiting for ' + what);
		await wait(250);
	}
}

exports.run = async function () {
	const root = vscode.workspace.workspaceFolders[0].uri.fsPath;
	const ext = vscode.extensions.getExtension('unfield.tern');
	await ext.activate();

	// Routes: the provider lists them, through the language server.
	const page = path.join(root, 'src/routes/links/id.param.integer/+page.svelte');
	const doc = await vscode.workspace.openTextDocument(page);
	await vscode.window.showTextDocument(doc);

	// Code lens from the page to its load.
	const lenses = await until('code lenses', async () => {
		const l = await vscode.commands.executeCommand('vscode.executeCodeLensProvider', doc.uri);
		return l && l.find((x) => x.command && x.command.title.startsWith('Go:')) && l;
	});
	assert.match(lenses[0].command.title, /^Go: page\.server\.go:\d+$/);

	// Go to definition on data.
	const line = doc.getText().split('\n').findIndex((l) => l.includes('let { data }'));
	const col = doc.lineAt(line).text.indexOf('data') + 1;
	const defs = await vscode.commands.executeCommand('vscode.executeDefinitionProvider', doc.uri, new vscode.Position(line, col));
	assert.ok(defs.some((d) => (d.uri || d.targetUri).fsPath.endsWith(path.join('id.param.integer', 'page.server.go'))), 'data -> page.server.go: ' + JSON.stringify(defs));

	// Workspace symbols: the routes.
	const syms = await vscode.commands.executeCommand('vscode.executeWorkspaceSymbolProvider', 'links');
	assert.ok(syms.some((s) => s.name === '/links/[id=integer]'), 'symbols: ' + syms.map((s) => s.name));

	// tern check's errors appear on save, and go when fixed.
	const text = doc.getText();
	const edit = new vscode.WorkspaceEdit();
	edit.insert(doc.uri, new vscode.Position(line + 1, 0), '\tconst oops: number = data.link.title;\n');
	await vscode.workspace.applyEdit(edit);
	await doc.save();
	const diags = await until('a tern check error', () => vscode.languages.getDiagnostics(doc.uri).find((d) => d.source === 'tern check' && /not assignable/.test(d.message)), 120000);
	assert.strictEqual(diags.range.start.line, line + 1);
	fs.writeFileSync(page, text);
	await vscode.commands.executeCommand('workbench.action.files.revert');
	await doc.save();
	await until('the error to clear', () => !vscode.languages.getDiagnostics(doc.uri).some((d) => d.source === 'tern check' && /not assignable/.test(d.message)), 120000);

	// Alt+O's command opens the other half.
	await vscode.commands.executeCommand('tern.toggle');
	await until('page.server.go to open', () => vscode.window.activeTextEditor && vscode.window.activeTextEditor.document.fileName.endsWith('page.server.go'));
	console.log('tern extension: all checks passed');
};
