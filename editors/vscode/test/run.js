// Runs test/suite.js inside a downloaded VS Code, with Shelf open.
const path = require('node:path');
const { runTests } = require('@vscode/test-electron');

runTests({
	extensionDevelopmentPath: path.resolve(__dirname, '..'),
	extensionTestsPath: path.resolve(__dirname, 'suite.js'),
	launchArgs: [path.resolve(__dirname, '../../../examples/shelf'), '--disable-extensions']
}).catch((err) => {
	console.error(err);
	process.exit(1);
});
