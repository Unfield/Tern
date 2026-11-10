import net from 'node:net';

/** A minimal SMTP server for tests: it accepts every message and keeps it. */
export async function smtpSink() {
	const messages: string[] = [];
	const server = net.createServer((socket) => {
		socket.setEncoding('utf8');
		let data = false;
		let current = '';
		let buffer = '';
		const say = (line: string) => socket.write(line + '\r\n');
		say('220 sink ESMTP');
		socket.on('data', (chunk: string) => {
			buffer += chunk;
			let i: number;
			while ((i = buffer.indexOf('\r\n')) >= 0) {
				const line = buffer.slice(0, i);
				buffer = buffer.slice(i + 2);
				if (data) {
					if (line === '.') {
						data = false;
						messages.push(current);
						current = '';
						say('250 queued');
					} else current += line.replace(/^\.\./, '.') + '\r\n';
					continue;
				}
				const cmd = line.toUpperCase();
				if (cmd.startsWith('EHLO') || cmd.startsWith('HELO')) {
					socket.write('250-sink\r\n250 8BITMIME\r\n');
				} else if (cmd === 'DATA') {
					data = true;
					say('354 go ahead');
				} else if (cmd === 'QUIT') {
					say('221 bye');
					socket.end();
				} else say('250 ok');
			}
		});
	});
	await new Promise<void>((resolve) => server.listen(0, '127.0.0.1', resolve));
	const port = (server.address() as net.AddressInfo).port;
	return { port, messages, close: () => new Promise((r) => server.close(r)) };
}
