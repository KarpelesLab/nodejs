/**
 * HTTP Solution for Node.js Integration with Go
 * 
 * This module provides a solution for using Node.js 22+ native Response objects
 * in a way that works correctly with Go's HTTP handler integration. It handles
 * both global handlers and VM context handlers.
 * 
 * Key concepts:
 * 1. Monkey-patches platform.emit to ensure Buffer chunks are converted to strings
 * 2. Provides helper functions for creating Response objects
 * 3. Supports both global handlers and VM context handlers
 * 4. Maintains state persistence across requests
 */

// ----- SECTION 1: Fix for Buffer-to-String Transmission -----

// Monkey patch platform.emit to fix body transmission
const originalEmit = platform.emit;
platform.emit = function(event, data) {
  // Only intercept response events with chunks
  if (event === 'send' && data.action === 'response' && data.data && data.data.chunk) {
    // If chunk is a Buffer, convert it to a string to ensure proper transmission
    if (Buffer.isBuffer(data.data.chunk)) {
      data.data.chunk = data.data.chunk.toString('utf8');
    }
  }
  return originalEmit.apply(this, arguments);
};

// ----- SECTION 2: Helper Functions for Response Creation -----

// Helper for text responses
function textResponse(text, status = 200) {
  return new Response(text, {
    status: status,
    headers: { 'Content-Type': 'text/plain' }
  });
}

// Helper for JSON responses
function jsonResponse(data, status = 200) {
  return new Response(JSON.stringify(data), {
    status: status,
    headers: { 'Content-Type': 'application/json' }
  });
}

// ----- SECTION 3: Global Handler Implementations -----

// Simple text handler
global.textHandler = function(request) {
  return textResponse("Hello from the text handler!");
};

// Simple JSON handler
global.jsonHandler = function(request) {
  return jsonResponse({
    message: "Hello JSON",
    timestamp: new Date().toISOString(),
    url: request.url,
    method: request.method
  });
};

// Counter with state
let counter = 0;

global.getCounter = function(request) {
  return jsonResponse({ value: counter });
};

global.incrementCounter = function(request) {
  counter++;
  return jsonResponse({ value: counter });
};

global.resetCounter = function(request) {
  counter = 0;
  return jsonResponse({ value: counter });
};

// User management
const users = [
  { id: 1, name: "Alice", email: "alice@example.com" },
  { id: 2, name: "Bob", email: "bob@example.com" },
  { id: 3, name: "Charlie", email: "charlie@example.com" }
];

global.listUsers = function(request) {
  return jsonResponse(users);
};

global.getUser = function(request) {
  try {
    const url = new URL(request.url);
    const id = parseInt(url.searchParams.get("id"), 10);
    
    if (isNaN(id)) {
      return jsonResponse({ error: "Invalid ID parameter" }, 400);
    }
    
    const user = users.find(u => u.id === id);
    
    if (!user) {
      return jsonResponse({ error: "User not found" }, 404);
    }
    
    return jsonResponse(user);
  } catch (e) {
    return jsonResponse({ error: e.message }, 500);
  }
};

global.createUser = async function(request) {
  try {
    const bodyText = await request.text();
    const data = JSON.parse(bodyText);
    
    if (!data.name || !data.email) {
      return jsonResponse({ error: "Name and email are required" }, 400);
    }
    
    const id = users.length > 0 
      ? Math.max(...users.map(u => u.id)) + 1 
      : 1;
    
    const newUser = {
      id,
      name: data.name,
      email: data.email
    };
    
    users.push(newUser);
    
    return jsonResponse(newUser, 201);
  } catch (e) {
    return jsonResponse({ error: e.message }, 500);
  }
};

// ----- SECTION 4: VM Context Management -----

// Storage for context information
const contextRegistry = {};

// Function to create a context
function createContext(name, initCode = '') {
  console.log(`Creating context: ${name}`);
  
  // Check if already exists
  if (contextRegistry[name]) {
    console.log(`Context already exists: ${name}`);
    return false;
  }
  
  // Create VM context
  platform.emit('create_context', {ctxid: name});
  
  // Initialize context with helper functions
  platform.emit('eval_in_context', {
    ctxid: name,
    data: `
      // Initialize state
      this.state = {};
      
      // Helper functions
      this.textResponse = function(text, status = 200) {
        return {
          status: status,
          headers: { 'Content-Type': 'text/plain' },
          body: text
        };
      };
      
      this.jsonResponse = function(data, status = 200) {
        return {
          status: status,
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(data)
        };
      };
      
      // Custom initialization code
      ${initCode}
    `
  });
  
  // Register context
  contextRegistry[name] = {
    name: name,
    created: Date.now(),
    handlers: {}
  };
  
  return true;
}

// Function to register a handler in a context
function registerHandler(contextName, handlerName, handlerCode) {
  console.log(`Registering handler: ${contextName}.${handlerName}`);
  
  // Create context if needed
  if (!contextRegistry[contextName]) {
    createContext(contextName);
  }
  
  // Define handler in context
  platform.emit('eval_in_context', {
    ctxid: contextName,
    data: `this.${handlerName} = ${handlerCode.toString()};`
  });
  
  // Create global handler for the context
  const fullHandlerName = `${contextName}.${handlerName}`;
  
  // Define global function to handle requests
  global[fullHandlerName] = function(request) {
    // Return a promise that will resolve to a Response
    return new Promise((resolve, reject) => {
      // Create a unique execution ID
      const execId = `exec-${Date.now()}-${Math.random().toString(36).slice(2)}`;
      
      // Set up response handler
      const responseHandler = function(event) {
        if (event.action === 'response' && event.data && event.data.id === execId) {
          // Remove listener and timeout
          platform.removeListener('in', responseHandler);
          clearTimeout(timeoutId);
          
          // Check for errors
          if (event.data.error) {
            console.log(`Error in context handler: ${event.data.error}`);
            reject(new Error(event.data.error));
            return;
          }
          
          try {
            // Process response from context
            const result = event.data.res;
            
            // Create a proper Response object
            resolve(new Response(
              result.body || '',
              {
                status: result.status || 200,
                headers: result.headers || {}
              }
            ));
          } catch (e) {
            console.log(`Error processing context result: ${e.toString()}`);
            reject(e);
          }
        }
      };
      
      // Set timeout for response
      const timeoutId = setTimeout(() => {
        platform.removeListener('in', responseHandler);
        reject(new Error('Handler timeout'));
      }, 5000);
      
      // Listen for response
      platform.on('in', responseHandler);
      
      // First read the request body
      if (typeof request.text === 'function') {
        request.text().then(bodyText => {
          executeInContext(bodyText);
        }).catch(error => {
          console.log(`Error reading request body: ${error}`);
          reject(error);
        });
      } else {
        // If request.text() is not available, use body directly or empty string
        executeInContext(request.body || '');
      }
      
      // Helper to execute in context
      function executeInContext(bodyText) {
        // Create request object to pass to the context
        const requestObj = {
          method: request.method,
          url: request.url,
          path: request.path || (request.url ? new URL(request.url).pathname : ''),
          query: request.query || (request.url ? new URL(request.url).search.substr(1) : ''),
          headers: {},
          body: bodyText
        };
        
        // Copy headers
        for (const [key, value] of Object.entries(request.headers || {})) {
          requestObj.headers[key] = value;
        }
        
        // Create code to execute in context
        const code = `
          (async function() {
            try {
              // Create request object for the handler
              const req = {
                method: ${JSON.stringify(requestObj.method)},
                url: ${JSON.stringify(requestObj.url)},
                path: ${JSON.stringify(requestObj.path)},
                query: ${JSON.stringify(requestObj.query)},
                headers: ${JSON.stringify(requestObj.headers)},
                body: ${JSON.stringify(requestObj.body)},
                text: function() { return Promise.resolve(${JSON.stringify(requestObj.body)}); },
                json: function() { 
                  try {
                    return Promise.resolve(JSON.parse(${JSON.stringify(requestObj.body)}));
                  } catch (e) {
                    return Promise.reject(e);
                  }
                }
              };
              
              // Call the handler
              const response = await this.${handlerName}(req);
              
              // Validate response
              if (!response || typeof response !== 'object') {
                throw new Error('Handler must return a response object');
              }
              
              // Handle the case where the body is an object that needs to be stringified
              let body = response.body;
              if (typeof body === 'object' && body !== null) {
                body = JSON.stringify(body);
              }
              
              // Ensure it has the correct properties
              return {
                status: response.status || 200,
                headers: response.headers || {},
                body: body || ''
              };
            } catch (e) {
              console.log("Error in context handler:", e.toString());
              throw e;
            }
          })();
        `;
        
        // Execute in context
        platform.emit('eval_in_context', {
          ctxid: contextName,
          id: execId,
          data: code
        });
      }
    });
  };
  
  // Register handler in registry
  contextRegistry[contextName].handlers[handlerName] = {
    name: handlerName,
    registered: Date.now()
  };
  
  return fullHandlerName;
}

// Function to list contexts
function listContexts() {
  const result = {};
  
  for (const name in contextRegistry) {
    const context = contextRegistry[name];
    const handlers = Object.keys(context.handlers || {});
    
    result[name] = {
      created: context.created,
      handlers: handlers
    };
  }
  
  return result;
}

// Function to remove a context
function removeContext(name) {
  if (!contextRegistry[name]) {
    console.log(`Context does not exist: ${name}`);
    return false;
  }
  
  console.log(`Removing context: ${name}`);
  
  // Get handlers
  const handlers = contextRegistry[name].handlers || {};
  
  // Remove global handlers
  for (const handlerName in handlers) {
    const fullName = `${name}.${handlerName}`;
    delete global[fullName];
  }
  
  // Remove context
  platform.emit('free_context', {ctxid: name});
  delete contextRegistry[name];
  
  console.log(`Context removed successfully: ${name}`);
  return true;
}

// ----- SECTION 5: Export API to global scope -----

global.createContextWithHandler = createContext;
global.registerContextHandler = registerHandler;
global.removeContextWithHandlers = removeContext;
global.listContextsWithHandlers = listContexts;

console.log("Node.js HTTP solution loaded successfully!");