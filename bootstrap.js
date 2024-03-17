(function() {
	const EventEmitter = require('events');
	const util = require('util');
	const vm = require('vm');

	//const importModuleDynamically = function(specifier, script, importAttributes) { ... };

	const platform = function() {
		let pf = new EventEmitter();
		let buff = "";
		let ipcid = 0;
		let ipccb = {};

		pf.on('send', data => process.stdout.write(JSON.stringify(data)+'\n'));
		pf.on('raw_in', data => pf.emit('in', JSON.parse(data)));
		pf.on('in', data => pf.emit(data.action, data));
		pf.on('exit', () => process.exit(0));
		pf.on('eval', async (msg) => {
			try {
				if (msg.opts.filename.endsWith(".mjs")) {
					let mod = new vm.SourceTextModule(msg.data, msg.opts);
					await mod.link(() => {});
					await mod.evaluate();
					global.exports = mod.namespace;
					if (msg.id) pf.emit('send', {'action': 'response', data: {id: msg.id, res: true}});
				} else {
					let res = vm.runInThisContext(msg.data, msg.opts);
					if (msg.id) pf.emit('send', {'action': 'response', data: {id: msg.id, res}});
				}
			} catch(e) {
				console.log(e);
				if (msg.id) pf.emit('send', {'action': 'response', data: {id: msg.id, error: e.toString()}});
			}
		});
		pf.on('set', (msg) => { global[msg.key] = msg.data; });
		pf.on('response', (msg) => pf.emit('send', {'action': 'response', ...msg}));
		// ipc stuff
		pf.on('ipc', (data) => {ipccb[ipcid]=data.cb;pf.emit('send', {'action':'ipc.req','id':ipcid++,'data': data.args})});
		pf.on('ipc.success', (msg) => {let v=ipccb[msg.id]; delete ipccb[msg.id]; v.ok(msg.data);});
		pf.on('ipc.failure', (msg) => {let v=ipccb[msg.id]; delete ipccb[msg.id]; v.fail(msg.data);});

		process.stdin.on('data', data => {
			buff += data;
			lines = buff.split(/\n/);
			buff = lines.pop();
			lines.forEach(line => pf.emit('raw_in', line));
		})
		.on('end', () => {
			if (buff.length > 0) pf.emit('raw_in', buff);
			pf.emit('end');
			process.exit(0);
		});

		return pf;
	}();

	// expose platform in global
	global.platform = platform;

	global.platform_ipc = function(cmd) {
		return new Promise((ok, fail) => global.platform.emit('ipc', {cb: {ok,fail}, args: cmd}));
	};
	// old callback-based methods
	global.__platformRest = function(name, verb, params, callback) {
		global.platform_ipc({ipc: 'rest', name, verb, params}).then(res => callback(res, undefined)).catch(res => callback(undefined, res));
	};
	global.__platformSetCookie = function(name, value, expiration) {
		global.platform_ipc({ipc: 'set_cookie', name, value, expiration}).catch(e => {});
	};

	// new promise based APIs
	global.__platformAsyncRest = (name, verb, params, context) => global.platform_ipc({ipc: 'rest', name, verb, params, context});

	// override console.log
	console.log = function() {
		platform.emit('send', {'action': 'console.log', 'data': util.format.apply(this, arguments)});
	};
	process.on('uncaughtException', (err, origin) => {
		//platform.emit('send', {'action': 'exception', 'data': `Caught exception: ${err}\nException origin: ${origin}`});
		console.log(`Caught exception: ${err}\nException origin: ${origin}`);
		process.exit(1);
	});
	process.on('unhandledRejection', (reason, promise) => {
		console.log('Unhandled Rejection at:', promise, 'reason:', reason);
		process.exit(1);
	});

	platform.emit('send', {'action': 'versions', 'data': process.versions});
	global.platform_ipc({ipc: 'version'}).then((msg) => {
		//console.log("bootstrap running nodejs "+process.version+" against "+msg.version);
		platform.emit('send', {'action': 'ready'})
	});
})();
