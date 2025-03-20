/**
 * Bootstrap script for NodeJS integration with Go.
 * This script sets up communication channels, event handling, and virtual context support
 * to allow Go code to execute JavaScript in a controlled environment.
 */
(function() {
	const EventEmitter = require('events');
	const util = require('util');
	const vm = require('vm');
	const stream = require('stream');

	// Node.js 22+ includes native Fetch API classes:
	// - Headers
	// - Request
	// - Response
	
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

	const defaultContext = {
		// Make native fetch API classes available in contexts
		Headers: globalThis.Headers,
		Request: globalThis.Request,
		Response: globalThis.Response
	};

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
				msg.opts = msg.opts || {}; // Ensure opts exists
				if (msg.opts.filename && msg.opts.filename.endsWith(".mjs")) {
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
					if (msg.id) {
						Promise.resolve(res).then(value => {
							pf.emit('send', {
								'action': 'response',
								data: { id: msg.id, res: value }
							});
						}).catch(e => {
							pf.emit('send', {
								'action': 'response',
								data: { id: msg.id, error: e.toString() }
							});
						});
					}
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
					// Explicitly add Fetch API classes
					Response: globalThis.Response,
					Headers: globalThis.Headers,
					Request: globalThis.Request,
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
				
				msg.opts = msg.opts || {}; // Ensure opts exists

				// If it's .mjs, treat it like a module
				if (msg.opts.filename && msg.opts.filename.endsWith(".mjs")) {
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
					if (msg.id) {
						Promise.resolve(res).then(value => {
							pf.emit('send', {
								'action': 'response',
								data: { id: msg.id, res: value }
							});
						}).catch(e => {
							pf.emit('send', {
								'action': 'response',
								data: { id: msg.id, error: e.toString() }
							});
						});
					}
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

		// ---- HTTP SUPPORT ----
		// Handle HTTP requests from Go
		pf.on('http.request', async (msg) => {
			const { id, reqID, handler, data } = msg;
			
			// Find the handler function
			let handlerFunction = null;
			
			// Check if handler name contains a dot (context.handler)
			if (handler.includes('.')) {
				const parts = handler.split('.');
				const contextName = parts[0];
				const methodName = parts[1];
				
				// Check if the context exists
				if (contexts[contextName]) {
					const contextObj = contexts[contextName].sandbox;
					handlerFunction = contextObj[methodName];
				}
			} else {
				// Look for the handler in the global scope
				handlerFunction = global[handler];
			}

			// Check if the handler exists
			if (typeof handlerFunction !== 'function') {
				pf.emit('send', {
					'action': 'response',
					data: { id, error: `HTTP handler '${handler}' not found or is not a function` }
				});
				return;
			}

			// Create a Request object using native Request from nodejs
			const request = new Request(data.url, {
				method: data.method,
				headers: data.headers,
				body: data.body ? Buffer.from(data.body) : null
			});
			
			// Add extra properties from Go request that aren't in standard Request
			request.path = data.path;
			request.query = data.query;
			request.remoteAddr = data.remoteAddr;
			request.host = data.host;

			try {
				// Call the handler with the Request object and get a Response
				const responsePromise = Promise.resolve(handlerFunction(request));
				const response = await responsePromise;
				
				// Validate the response
				console.log("Response check:", response?.constructor?.name, typeof response);
				// Due to JavaScript context differences, instanceof may not work across contexts
				// Instead, check if it has the expected Response properties
				if (!response || typeof response !== 'object' || 
					typeof response.headers !== 'object' || 
					typeof response.status !== 'number') {
					throw new Error('Handler must return a Response object');
				}
				console.log("Response is valid");
				console.log("Response details:", JSON.stringify({
					status: response.status,
					bodyType: typeof response.body,
					headersType: typeof response.headers
				}));
				
				// Send headers to Go
				// Convert headers to object (handle different Headers implementations)
				const headersObj = {};
				// Extract headers from the native Headers object or a plain object
				if (typeof response.headers.forEach === 'function') {
					// Standard Headers object with forEach method
					response.headers.forEach((value, key) => {
						headersObj[key] = value;
					});
				} else if (typeof response.headers === 'object') {
					// Plain object with headers as properties
					Object.assign(headersObj, response.headers);
				}
				
				console.log("Headers object:", headersObj);
				
				pf.emit('send', {
					'action': 'response',
					data: { 
						id: reqID + '.headers', 
						statusCode: response.status,
						headers: headersObj
					}
				});
				
				// Get response body as buffer and send it
				try {
					// First try text() since it's more reliable with standard Response
					if (typeof response.text === 'function') {
						// Use Response.text() method which returns a Promise of the body as text
						const text = await response.text();
						
						console.log("Body retrieved via text():", text.length, "Content:", JSON.stringify(text));
						
						// Send the text as a string directly to ensure proper transmission
						pf.emit('send', {
							'action': 'response',
							data: { 
								id: reqID + '.body',
								chunk: text
							}
						});
					} else if (typeof response.arrayBuffer === 'function') {
						// Try arrayBuffer method as backup
						const arrayBuffer = await response.arrayBuffer();
						
						console.log("Body retrieved via arrayBuffer():", arrayBuffer.byteLength);
						
						if (arrayBuffer.byteLength > 0) {
							const text = Buffer.from(arrayBuffer).toString('utf8');
							pf.emit('send', {
								'action': 'response',
								data: { 
									id: reqID + '.body',
									chunk: text
								}
							});
						}
					} else if (response._body !== undefined) {
						// Our custom Response implementation
						let text;
						if (typeof response._body === 'string') {
							text = response._body;
						} else {
							text = Buffer.from(response._body).toString('utf8');
						}
						
						console.log("Body retrieved via _body:", text.length);
						
						pf.emit('send', {
							'action': 'response',
							data: { 
								id: reqID + '.body',
								chunk: text
							}
						});
					} else if (response.body) {
						// Direct body property
						let text;
						if (typeof response.body === 'string') {
							text = response.body;
						} else {
							text = Buffer.from(response.body).toString('utf8');
						}
						
						console.log("Body retrieved via body property:", text.length);
						
						pf.emit('send', {
							'action': 'response',
							data: { 
								id: reqID + '.body',
								chunk: text
							}
						});
					}
				} catch (bodyError) {
					console.log(`Error getting response body: ${bodyError.toString()}`);
				}
				
				// Signal response completion
				pf.emit('send', {
					'action': 'response',
					data: { id: reqID + '.done' }
				});
				
				// Send success response to initial request
				pf.emit('send', {
					'action': 'response',
					data: { id: id, res: true }
				});
			} catch (e) {
				console.log(`HTTP handler error: ${e.toString()}`);
				
				// Send error notification
				pf.emit('send', {
					'action': 'response',
					data: { 
						id: reqID + '.error',
						error: e.toString()
					}
				});
				
				// Send error to initial request
				pf.emit('send', {
					'action': 'response',
					data: { id: id, error: e.toString() }
				});
			}
		});
		// ---- END HTTP SUPPORT ----

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
	defaultContext.__platformAsyncRest = global.__platformAsyncRest;

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