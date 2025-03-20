package nodejs_test

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/KarpelesLab/nodejs"
)

func Example_contextBasic() {
	// Create a new NodeJS factory
	factory, err := nodejs.New()
	if err != nil {
		log.Fatalf("Failed to create factory: %v", err)
	}

	// Create a new NodeJS process
	proc, err := factory.New()
	if err != nil {
		log.Fatalf("Failed to create process: %v", err)
	}
	defer proc.Close()

	// Create a new JavaScript context
	jsCtx, err := proc.NewContext()
	if err != nil {
		log.Fatalf("Failed to create JavaScript context: %v", err)
	}
	defer jsCtx.Close()

	// Execute JavaScript in the context
	ctx := context.Background()
	result, err := jsCtx.Eval(ctx, "40 + 2", nil)
	if err != nil {
		log.Fatalf("Failed to evaluate code: %v", err)
	}

	fmt.Printf("Result: %v\n", result)
}

func Example_contextWithMultipleContexts() {
	// Create a new NodeJS factory
	factory, err := nodejs.New()
	if err != nil {
		log.Fatalf("Failed to create factory: %v", err)
	}

	// Create a new NodeJS process
	proc, err := factory.New()
	if err != nil {
		log.Fatalf("Failed to create process: %v", err)
	}
	defer proc.Close()

	// Create two separate contexts
	ctx1, err := proc.NewContext()
	if err != nil {
		log.Fatalf("Failed to create first JavaScript context: %v", err)
	}
	defer ctx1.Close()

	ctx2, err := proc.NewContext()
	if err != nil {
		log.Fatalf("Failed to create second JavaScript context: %v", err)
	}
	defer ctx2.Close()

	// Execute code in the first context
	goCtx := context.Background()
	_, err = ctx1.Eval(goCtx, "var counter = 1", nil)
	if err != nil {
		log.Fatalf("Failed to set variable in first context: %v", err)
	}

	// Execute code in the second context
	_, err = ctx2.Eval(goCtx, "var counter = 100", nil)
	if err != nil {
		log.Fatalf("Failed to set variable in second context: %v", err)
	}

	// Increment counter in first context
	_, err = ctx1.Eval(goCtx, "counter++", nil)
	if err != nil {
		log.Fatalf("Failed to increment counter in first context: %v", err)
	}

	// Get counter values from both contexts
	counter1, err := ctx1.Eval(goCtx, "counter", nil)
	if err != nil {
		log.Fatalf("Failed to get counter from first context: %v", err)
	}

	counter2, err := ctx2.Eval(goCtx, "counter", nil)
	if err != nil {
		log.Fatalf("Failed to get counter from second context: %v", err)
	}

	fmt.Printf("Counter in context 1: %v\n", counter1)
	fmt.Printf("Counter in context 2: %v\n", counter2)
}

func Example_contextWithTimeout() {
	// Create a new NodeJS factory
	factory, err := nodejs.New()
	if err != nil {
		log.Fatalf("Failed to create factory: %v", err)
	}

	// Create a new NodeJS process
	proc, err := factory.New()
	if err != nil {
		log.Fatalf("Failed to create process: %v", err)
	}
	defer proc.Close()

	// Create a JavaScript context
	jsCtx, err := proc.NewContext()
	if err != nil {
		log.Fatalf("Failed to create JavaScript context: %v", err)
	}
	defer jsCtx.Close()

	// Create a Go context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Try to execute long-running code
	_, err = jsCtx.Eval(ctx, `
		// This will run for a long time
		const start = Date.now();
		while (Date.now() - start < 5000) {
			// Do nothing, just waste time
		}
		return "Done!";
	`, nil)

	if err != nil {
		fmt.Printf("Execution failed as expected: %v\n", err)
	} else {
		fmt.Println("Execution completed, which was not expected")
	}
}

func Example_contextWithEvalChannel() {
	// Create a new NodeJS factory
	factory, err := nodejs.New()
	if err != nil {
		log.Fatalf("Failed to create factory: %v", err)
	}

	// Create a new NodeJS process
	proc, err := factory.New()
	if err != nil {
		log.Fatalf("Failed to create process: %v", err)
	}
	defer proc.Close()

	// Create a JavaScript context
	jsCtx, err := proc.NewContext()
	if err != nil {
		log.Fatalf("Failed to create JavaScript context: %v", err)
	}
	defer jsCtx.Close()

	// Use EvalChannel for asynchronous execution
	resultChan, err := jsCtx.EvalChannel(`
		// This function simulates a database query that takes time
		async function fetchUserData() {
			// Simple async operation that doesn't rely on setTimeout
			await Promise.resolve(); 
			return {
				id: 123,
				name: "John Doe",
				email: "john@example.com"
			};
		}
		
		// Call the async function and return its result
		fetchUserData();
	`, nil)

	if err != nil {
		log.Fatalf("Failed to start async evaluation: %v", err)
	}

	// Do other work while waiting for the result
	fmt.Println("Async evaluation started, doing other work...")

	// Wait for the result from the channel
	select {
	case result := <-resultChan:
		// Check for errors
		if errMsg, ok := result["error"].(string); ok {
			fmt.Printf("Error from JavaScript: %s\n", errMsg)
			return
		}

		// Process the result
		if userData, ok := result["res"].(map[string]interface{}); ok {
			fmt.Printf("User ID: %v\n", userData["id"])
			fmt.Printf("User Name: %s\n", userData["name"])
			fmt.Printf("User Email: %s\n", userData["email"])
		} else {
			fmt.Printf("Unexpected result type: %T\n", result["res"])
		}
	case <-time.After(1 * time.Second):
		fmt.Println("Timed out waiting for result")
	}
}
