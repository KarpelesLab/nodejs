(function() {
	const EventEmitter = require('events');
	const util = require('util');
	const vm = require('vm');

	// Basic linker for ES modules
	const basicLinker = async function(specifier) {
		switch(specifier) {
			case "node:stream":
				const mod = await import(specifier);
				const keys = Object.keys(mod);
				return new vm.SyntheticModule(
					[...keys],
					function init() {
						for (const k of keys)
							this.setExport(k, mod[k]);
					},
					{ identifier: 'synthetic:'+specifier },
				);
		}
		throw new Error(`Unknown import: ${specifier}`);
	};

	const defaultContext = {};

	// Top-level event-based "platform" object
	const platform = function() {
		let pf = new EventEmitter();
		let buff = "";
		let ipcid = 0;
		let ipccb = {};

		pf.on('send', data => process.stdout.write(JSON.stringify(data)+'\n'));
		pf.on('raw_in', data => pf.emit('in', JSON.parse(data)));
		pf.on('in', data => pf.emit(data.action, data));
		pf.on('exit', () => process.exit(0));

		// legacy code (global object)
		pf.on('eval', async (msg) => {
			try {
				if (msg.opts.filename.endsWith(".mjs")) {
					let mod = new vm.SourceTextModule(msg.data, msg.opts);
					await mod.link(basicLinker);
					await mod.evaluate();
					global.exports = mod.namespace;
					if (msg.id) pf.emit('send', {
						'action': 'response',
						data: { id: msg.id, res: true }
					});
				} else {
					let res = vm.runInThisContext(msg.data, msg.opts);
					if (msg.id) pf.emit('send', {
						'action': 'response',
						data: { id: msg.id, res }
					});
				}
			} catch(e) {
				console.log(e);
				if (msg.id) pf.emit('send', {
					'action': 'response',
					data: { id: msg.id, error: e.toString() }
				});
			}
		});

		pf.on('set', (msg) => { global[msg.key] = msg.data; });
		pf.on('response', (msg) => pf.emit('send', {'action': 'response', ...msg}));

		// IPC bridging
		pf.on('ipc', (data) => {
			ipccb[ipcid] = data.cb;
			pf.emit('send', {'action':'ipc.req','id':ipcid++,'data': data.args});
		});
		pf.on('ipc.success', (msg) => {
			let v = ipccb[msg.id];
			delete ipccb[msg.id];
			v.ok(msg.data);
		});
		pf.on('ipc.failure', (msg) => {
			let v = ipccb[msg.id];
			delete ipccb[msg.id];
			v.fail(msg.data);
		});

		// ---- CONTEXT SUPPORT ----
		//
		// We'll store contexts keyed by a string, with each entry containing:
		//   { sandbox: {…}, context: vm.createContext(...) }
		//
		const contexts = {};

		// CREATE a new named context
		pf.on('create_context', (msg) => {
			try {
				const ctxid = msg.ctxid;
				if (!ctxid) throw new Error("No ctxid provided.");
				if (contexts[ctxid]) throw new Error(`Context '${ctxid}' already exists.`);

				const sandbox = {
					...defaultContext,
					console: {
						log: function() {
							platform.emit('send', {'action': 'console.log', 'context': ctxid, 'data': util.format.apply(this, arguments)});
						},
					},
				};
				const context = vm.createContext(sandbox);

				// store it
				contexts[ctxid] = { sandbox, context };

				if (msg.id) pf.emit('send', {
					action: 'response',
					data: { id: msg.id, res: true }
				});
			} catch(e) {
				if (msg.id) pf.emit('send', {
					action: 'response',
					data: { id: msg.id, error: e.toString() }
				});
			}
		});

		// EVAL/RUN in an existing named context
		pf.on('eval_in_context', async (msg) => {
			try {
				const ctxid = msg.ctxid;
				if (!ctxid || !contexts[ctxid]) {
					throw new Error(`Context '${ctxid}' does not exist.`);
				}
				const { context } = contexts[ctxid];

				// If it's .mjs, treat it like a module
				if (msg.opts.filename.endsWith(".mjs")) {
					let mod = new vm.SourceTextModule(msg.data, { ...msg.opts, context });
					await mod.link(basicLinker);
					await mod.evaluate();
					// if you want the namespace, it is mod.namespace

					if (msg.id) pf.emit('send', {
						'action': 'response',
						data: { id: msg.id, res: true }
					});
				} else {
					// plain script
					let res = vm.runInContext(msg.data, context, msg.opts);
					if (msg.id) pf.emit('send', {
						'action': 'response',
						data: { id: msg.id, res }
					});
				}
			} catch(e) {
				if (msg.id) pf.emit('send', {
					action: 'response',
					data: { id: msg.id, error: e.toString() }
				});
			}
		});

		// FREE a named context
		pf.on('free_context', (msg) => {
			try {
				const ctxid = msg.ctxid;
				if (!ctxid) throw new Error("No ctxid provided.");
				delete contexts[ctxid];
				if (msg.id) pf.emit('send', {
					action: 'response',
					data: { id: msg.id, res: true }
				});
			} catch(e) {
				if (msg.id) pf.emit('send', {
					action: 'response',
					data: { id: msg.id, error: e.toString() }
				});
			}
		});
		// ---- END CONTEXT SUPPORT ----

		// Handle incoming data from stdin
		process.stdin.on('data', data => {
			buff += data;
			let lines = buff.split(/\n/);
			buff = lines.pop();
			lines.forEach(line => pf.emit('raw_in', line));
		}).on('end', () => {
			if (buff.length > 0) pf.emit('raw_in', buff);
			pf.emit('end');
			process.exit(0);
		});

		return pf;
	}();

	// Expose platform globally
	global.platform = platform;

	// Simple IPC promise helper
	global.platform_ipc = function(cmd) {
		return new Promise((ok, fail) => {
			global.platform.emit('ipc', { cb: { ok, fail }, args: cmd });
		});
	};

	// Some convenience wrappers
	global.__platformRest = function(name, verb, params, callback) {
		global.platform_ipc({ipc: 'rest', name, verb, params})
			.then(res => callback(res, undefined))
			.catch(res => callback(undefined, res));
	};
	global.__platformSetCookie = function(name, value, expiration) {
		global.platform_ipc({ipc: 'set_cookie', name, value, expiration})
			.catch(e => {});
	};
	global.__platformAsyncRest = (name, verb, params, context) =>
		global.platform_ipc({ipc: 'rest', name, verb, params, context});

	defaultContext.__platformRest = global.__platformRest;
	defaultContext.__platformSetCookie = global.__platformSetCookie;
	defaultContext.__platformAsyncRest = global.__platformSetCookie;

	// Override console.log so it is routed out via platform
	console.log = function() {
		platform.emit('send', {
			'action': 'console.log',
			'data': util.format.apply(this, arguments)
		});
	};

	process.on('uncaughtException', (err, origin) => {
		console.log(`Caught exception: ${err}\nException origin: ${origin}`);
		process.exit(1);
	});

	process.on('unhandledRejection', (reason, promise) => {
		console.log('Unhandled Rejection at:', promise, 'reason:', reason);
		process.exit(1);
	});

	platform.emit('send', {'action': 'versions', 'data': process.versions});

	global.platform_ipc({ipc: 'version'}).then((msg) => {
		platform.emit('send', {'action': 'ready'});
	});
})();
