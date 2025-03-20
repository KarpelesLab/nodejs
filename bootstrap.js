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

	// ---- Web API Compatible Classes ----
	// Request and Response classes compatible with the Fetch API
	
	// Headers class implementation similar to the Web API
	class Headers {
		constructor(init) {
			this._headers = new Map();
			
			if (init) {
				if (init instanceof Headers) {
					for (const [key, value] of init._headers.entries()) {
						this.set(key, value);
					}
				} else if (Array.isArray(init)) {
					init.forEach(([key, value]) => this.append(key, value));
				} else if (typeof init === 'object') {
					Object.entries(init).forEach(([key, value]) => this.set(key, value));
				}
			}
		}
		
		append(name, value) {
			name = name.toLowerCase();
			if (!this._headers.has(name)) {
				this.set(name, value);
			} else {
				let current = this._headers.get(name);
				if (!Array.isArray(current)) {
					current = [current];
				}
				current.push(value);
				this._headers.set(name, current);
			}
		}
		
		delete(name) {
			this._headers.delete(name.toLowerCase());
		}
		
		get(name) {
			const value = this._headers.get(name.toLowerCase());
			if (Array.isArray(value)) {
				return value.join(', ');
			}
			return value || null;
		}
		
		has(name) {
			return this._headers.has(name.toLowerCase());
		}
		
		set(name, value) {
			this._headers.set(name.toLowerCase(), value);
		}
		
		forEach(callback, thisArg) {
			for (const [name, value] of this._headers.entries()) {
				callback.call(thisArg, value, name, this);
			}
		}
		
		// Convert to object for serialization
		toObject() {
			const obj = {};
			this._headers.forEach((value, key) => {
				obj[key] = value;
			});
			return obj;
		}
	}
	
	// Body mixin implementation
	const BodyMixin = {
		arrayBuffer() {
			if (this._bodyInitialized) {
				return Promise.resolve(this._body);
			}
			return Promise.resolve(Buffer.from(this._body || ''));
		},
		
		text() {
			if (this._bodyInitialized && typeof this._body === 'string') {
				return Promise.resolve(this._body);
			}
			return this.arrayBuffer()
				.then(buffer => buffer.toString());
		},
		
		json() {
			return this.text()
				.then(text => JSON.parse(text));
		},
		
		_initBody(body) {
			this._bodyInitialized = true;
			this._body = body || null;
		}
	};
	
	// Request class implementation
	class Request {
		constructor(input, init = {}) {
			// Initialize with defaults
			this.method = 'GET';
			this.headers = new Headers();
			this.redirect = 'follow';
			this.signal = null;
			this._initBody(null);
			
			// Process input
			if (input instanceof Request) {
				this.method = input.method;
				this.headers = new Headers(input.headers);
				this.url = input.url;
				this._initBody(input._body);
			} else if (typeof input === 'string') {
				this.url = input;
			} else if (typeof input === 'object') {
				Object.assign(this, input);
				if (input.headers) {
					this.headers = new Headers(input.headers);
				}
				if (input.body) {
					this._initBody(input.body);
				}
			}
			
			// Override with init
			if (init) {
				if (init.method) this.method = init.method;
				if (init.headers) this.headers = new Headers(init.headers);
				if (init.body !== undefined) this._initBody(init.body);
				if (init.redirect) this.redirect = init.redirect;
				if (init.signal) this.signal = init.signal;
			}
			
			// Uppercase method
			this.method = this.method.toUpperCase();
		}
	}
	
	// Add Body methods to Request.prototype
	Object.assign(Request.prototype, BodyMixin);
	
	// Response class implementation
	class Response {
		constructor(body = null, init = {}) {
			this.status = init.status || 200;
			this.statusText = init.statusText || '';
			this.headers = new Headers(init.headers);
			this.ok = this.status >= 200 && this.status < 300;
			this.type = 'default';
			this.url = init.url || '';
			this._initBody(body);
		}
		
		clone() {
			return new Response(this._body, {
				status: this.status,
				statusText: this.statusText,
				headers: new Headers(this.headers),
				url: this.url
			});
		}
	}
	
	// Add Body methods to Response.prototype
	Object.assign(Response.prototype, BodyMixin);
	
	// Static methods for Response
	Response.error = () => new Response(null, { status: 0, statusText: '' });
	Response.redirect = (url, status = 302) => {
		const response = new Response(null, { status });
		response.headers.set('Location', url);
		return response;
	};
	
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
			case "web-api":
				// Create a synthetic module for Web API compatibility
				return new vm.SyntheticModule(
					['Request', 'Response', 'Headers'],
					function init() {
						this.setExport('Request', Request);
						this.setExport('Response', Response);
						this.setExport('Headers', Headers);
					},
					{ identifier: 'synthetic:web-api' },
				);
		}
		throw new Error(`Unknown import: ${specifier}`);
	};

	const defaultContext = {
		Request,
		Response,
		Headers
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

			// Create a Request object compatible with Fetch API
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
				if (!(response instanceof Response)) {
					throw new Error('Handler must return a Response object');
				}
				
				// Send headers to Go
				pf.emit('send', {
					'action': 'response',
					data: { 
						id: reqID + '.headers', 
						statusCode: response.status,
						headers: response.headers.toObject()
					}
				});
				
				// Get response body as buffer and send it
				const body = await response.arrayBuffer();
				if (body && body.length > 0) {
					pf.emit('send', {
						'action': 'response',
						data: { 
							id: reqID + '.body',
							chunk: Buffer.from(body)
						}
					});
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