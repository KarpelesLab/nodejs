'use strict';

/**
 * Bootstrap script for NodeJS integration with Go.
 * This script sets up communication channels, event handling, and virtual context support
 * to allow Go code to execute JavaScript in a controlled environment.
 */
(function() {
	'use strict';
	
	const EventEmitter = require('events');
	const util = require('util');
	const vm = require('vm');
	const stream = require('stream');

	// Node.js 22+ includes native Fetch API classes:
	// - Headers
	// - Request
	// - Response
	
	// Basic linker for ES modules
	const basicLinker = async (specifier) => {
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
					{ identifier: `synthetic:${specifier}` },
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
		const pf = new EventEmitter();
		let buff = "";
		let ipcid = 0;
		const ipccb = {};

		pf.on('send', data => process.stdout.write(JSON.stringify(data)+'\n'));
		pf.on('raw_in', data => pf.emit('in', JSON.parse(data)));
		pf.on('in', data => pf.emit(data.action, data));
		pf.on('exit', () => process.exit(0));

		// legacy code (global object)
		pf.on('eval', async (msg) => {
			try {
				msg.opts = msg.opts || {}; // Ensure opts exists
				if (msg.opts.filename && msg.opts.filename.endsWith(".mjs")) {
					const mod = new vm.SourceTextModule(msg.data, msg.opts);
					await mod.link(basicLinker);
					await mod.evaluate();
					global.exports = mod.namespace;
					if (msg.id) pf.emit('send', {
						'action': 'response',
						data: { id: msg.id, res: true }
					});
				} else {
					const res = vm.runInThisContext(msg.data, msg.opts);
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
			const v = ipccb[msg.id];
			delete ipccb[msg.id];
			v.ok(msg.data);
		});
		pf.on('ipc.failure', (msg) => {
			const v = ipccb[msg.id];
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
						log: (...args) => {
							platform.emit('send', {'action': 'console.log', 'context': ctxid, 'data': util.format(...args)});
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
					const mod = new vm.SourceTextModule(msg.data, { ...msg.opts, context });
					await mod.link(basicLinker);
					await mod.evaluate();
					// if you want the namespace, it is mod.namespace

					if (msg.id) pf.emit('send', {
						'action': 'response',
						data: { id: msg.id, res: true }
					});
				} else {
					// plain script
					const res = vm.runInContext(msg.data, context, msg.opts);
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
			const { id, reqID, handler, data, context: contextId } = msg;
			
			// Find the handler function
			let handlerFunction = null;
			
			// If contextId is provided in the options, use it directly
			if (contextId && contexts[contextId]) {
				// Get handler from the specified context
				const contextObj = contexts[contextId].sandbox;
				handlerFunction = contextObj[handler];
			} else if (handler.includes('.')) {
				// Legacy support: parse context.method format
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
				if (!response || typeof response !== 'object' || 
					typeof response.headers !== 'object' || 
					typeof response.status !== 'number') {
					throw new Error('Handler must return a Response object');
				}

				// Process the response in a reliable order to prevent race conditions
				try {
					// 1. Always try to get complete response body first
					let responseText = null;
					
					// Get the response body BEFORE sending any messages
					if (typeof response.clone === 'function') {
						// Create a clone to safely extract the body text
						const clonedResponse = response.clone();
						if (typeof clonedResponse.text === 'function') {
							try {
								responseText = await clonedResponse.text();
							} catch (err) {
								// Fallback to other methods
							}
						}
					}
					
					// If we couldn't get the text from clone, try other methods
					if (responseText === null) {
						if (typeof response.text === 'function') {
							try {
								responseText = await response.text();
							} catch (err) {
								// Try next method
							}
						} else if (typeof response.arrayBuffer === 'function') {
							try {
								const buf = await response.arrayBuffer();
								responseText = Buffer.from(buf).toString('utf8');
							} catch (err) {
								// No more fallbacks
							}
						}
					}
					
					// 2. Extract headers from the Response object
					const headersObj = {};
					if (typeof response.headers.forEach === 'function') {
						response.headers.forEach((value, key) => {
							headersObj[key] = value;
						});
					} else if (typeof response.headers === 'object') {
						Object.assign(headersObj, response.headers);
					}
					
					// 3. Now send messages in the correct order:
					
					// Send headers first
					pf.emit('send', {
						'action': 'response',
						data: { 
							id: reqID + '.headers', 
							statusCode: response.status,
							headers: headersObj
						}
					});
					
					// Small delay to ensure headers are processed
					await new Promise(resolve => setTimeout(resolve, 5));
					
					// 4. Send body content if we have it
					if (responseText && responseText.length > 0) {
						pf.emit('send', {
							'action': 'response',
							data: { 
								id: reqID + '.body',
								chunk: Buffer.from(responseText, "utf8").toString("base64")
							}
						});
						
						// Small delay to ensure body is processed
						await new Promise(resolve => setTimeout(resolve, 5));
					}
					
					// 5. Signal completion
					pf.emit('send', {
						'action': 'response',
						data: { id: reqID + '.done' }
					});
					
					// 6. Wait for previous messages to be processed
					await new Promise(resolve => setTimeout(resolve, 30));
					
					// 7. Final success response
					pf.emit('send', {
						'action': 'response',
						data: { id: id, res: true }
					});
				} catch (bodyError) {
					throw bodyError;
				}
				
			} catch (e) {
				// Send error in the correct order
				
				// First send error notification
				pf.emit('send', {
					'action': 'response',
					data: { 
						id: reqID + '.error',
						error: e.toString()
					}
				});
				
				// Then send a done signal
				await new Promise(resolve => setTimeout(resolve, 5));
				pf.emit('send', {
					'action': 'response',
					data: { id: reqID + '.done' }
				});
				
				// Wait for previous messages to be processed
				await new Promise(resolve => setTimeout(resolve, 30));
				
				// Finally send the error to the initial request
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
			const lines = buff.split(/\n/);
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
	global.platform_ipc = (cmd) => {
		return new Promise((ok, fail) => {
			global.platform.emit('ipc', { cb: { ok, fail }, args: cmd });
		});
	};

	// Some convenience wrappers
	global.__platformRest = (name, verb, params, callback) => {
		global.platform_ipc({ipc: 'rest', name, verb, params})
			.then(res => callback(res, undefined))
			.catch(res => callback(undefined, res));
	};
	global.__platformSetCookie = (name, value, expiration) => {
		global.platform_ipc({ipc: 'set_cookie', name, value, expiration})
			.catch(e => {});
	};
	global.__platformAsyncRest = (name, verb, params, context) =>
		global.platform_ipc({ipc: 'rest', name, verb, params, context});

	defaultContext.__platformRest = global.__platformRest;
	defaultContext.__platformSetCookie = global.__platformSetCookie;
	defaultContext.__platformAsyncRest = global.__platformAsyncRest;

	// Override console.log so it is routed out via platform
	console.log = (...args) => {
		platform.emit('send', {
			'action': 'console.log',
			'data': util.format(...args)
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