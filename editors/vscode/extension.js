// Tern for VS Code: starts `go tool tern lsp` in each Tern project of the
// workspace, which jumps between pages and their Go loads, lists the routes
// and reports tern check's errors when a file is saved.
const path = require('node:path');
const fs = require('node:fs');
const vscode = require('vscode');
const { LanguageClient } = require('vscode-languageclient/node');

/** @type {Map<string, LanguageClient>} by project root */
const clients = new Map();

/** The project a file belongs to: the nearest directory with go.mod and src/routes. */
function projectOf(file) {
	for (let dir = path.dirname(file); dir !== path.dirname(dir); dir = path.dirname(dir)) {
		if (fs.existsSync(path.join(dir, 'go.mod')) && fs.existsSync(path.join(dir, 'src', 'routes'))) return dir;
	}
	return null;
}

async function start(root) {
	if (clients.has(root)) return clients.get(root);
	const [command, ...args] = vscode.workspace.getConfiguration('tern').get('server');
	const pattern = new vscode.RelativePattern(root, '**/*');
	const client = new LanguageClient(
		`tern:${root}`,
		`Tern (${path.basename(root)})`,
		{ command, args, options: { cwd: root } },
		{
			// By file, not language: .svelte files have no language without the Svelte extension.
			documentSelector: [{ scheme: 'file', pattern: root.split(path.sep).join('/') + '/**/*.{svelte,go,ts,js}' }],
			workspaceFolder: { uri: vscode.Uri.file(root), name: path.basename(root), index: 0 },
			synchronize: { fileEvents: vscode.workspace.createFileSystemWatcher(pattern) }
		}
	);
	clients.set(root, client);
	await client.start();
	return client;
}

async function startAll() {
	const found = await vscode.workspace.findFiles('**/src/routes/**/+*.svelte', '**/node_modules/**', 200);
	const roots = new Set(found.map((u) => projectOf(u.fsPath)).filter(Boolean));
	for (const root of roots) await start(root);
	await vscode.commands.executeCommand('setContext', 'tern.active', roots.size > 0);
	routes.refresh();
}

async function open(uri, line) {
	const doc = await vscode.workspace.openTextDocument(vscode.Uri.parse(uri));
	const pos = new vscode.Position(line ?? 0, 0);
	await vscode.window.showTextDocument(doc, { selection: new vscode.Range(pos, pos) });
}

/** The Routes view: each project's routes, opening the page or the endpoint. */
class Routes {
	constructor() {
		this.changed = new vscode.EventEmitter();
		this.onDidChangeTreeData = this.changed.event;
	}
	refresh() {
		this.changed.fire(undefined);
	}
	getTreeItem(e) {
		return e;
	}
	async getChildren(parent) {
		if (!parent) {
			if (clients.size === 1) return this.routes([...clients.values()][0]);
			return [...clients.keys()].map((root) => {
				const item = new vscode.TreeItem(path.basename(root), vscode.TreeItemCollapsibleState.Expanded);
				item.root = root;
				return item;
			});
		}
		return this.routes(clients.get(parent.root));
	}
	async routes(client) {
		const list = await client.sendRequest('tern/routes');
		return list.map((r) => {
			const item = new vscode.TreeItem(r.pattern);
			item.description = r.kind;
			item.iconPath = new vscode.ThemeIcon(r.kind === 'endpoint' ? 'symbol-interface' : 'file');
			const uri = r.page || r.server;
			if (uri) item.command = { command: 'tern.open', title: 'Open', arguments: [uri, r.page ? 0 : r.line] };
			item.tooltip = [r.page && 'Page: ' + vscode.Uri.parse(r.page).fsPath, r.server && 'Go: ' + vscode.Uri.parse(r.server).fsPath].filter(Boolean).join('\n');
			return item;
		});
	}
}
const routes = new Routes();

/** @param {vscode.ExtensionContext} context */
async function activate(context) {
	context.subscriptions.push(
		vscode.commands.registerCommand('tern.open', open),
		vscode.commands.registerCommand('tern.routes.refresh', () => routes.refresh()),
		vscode.commands.registerCommand('tern.restart', async () => {
			for (const c of clients.values()) await c.stop();
			clients.clear();
			await startAll();
		}),
		vscode.commands.registerCommand('tern.toggle', async () => {
			const editor = vscode.window.activeTextEditor;
			if (!editor) return;
			const root = projectOf(editor.document.uri.fsPath);
			const client = root && (await start(root));
			if (!client) return;
			const lenses = await client.sendRequest('textDocument/codeLens', { textDocument: { uri: editor.document.uri.toString() } });
			const to = lenses?.[0]?.command;
			if (to) await open(...to.arguments);
			else vscode.window.showInformationMessage('Tern: this file has no page or load to go to.');
		}),
		vscode.window.registerTreeDataProvider('ternRoutes', routes),
		vscode.workspace.onDidCreateFiles(() => routes.refresh()),
		vscode.workspace.onDidDeleteFiles(() => routes.refresh()),
		vscode.workspace.onDidRenameFiles(() => routes.refresh())
	);
	await startAll();
}

async function deactivate() {
	await Promise.all([...clients.values()].map((c) => c.stop()));
}

module.exports = { activate, deactivate };
