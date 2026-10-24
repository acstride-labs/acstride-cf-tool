package cli

import (
	"context"
	"encoding/json"
	"io"
	"testing"
	"time"

	"github.com/acstride-labs/acstride-cf-tool/internal/core"
)

func TestReadSessionStaysOpenAndRejectsWrites(t *testing.T) {
	input, writer := io.Pipe()
	reader, output := io.Pipe()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	app := &App{Client: core.New()}
	done := make(chan int, 1)
	go func() { done <- app.Run(ctx, []string{"serve"}, input, output, io.Discard); output.Close() }()
	encoder, decoder := json.NewEncoder(writer), json.NewDecoder(reader)
	for i, command := range []string{"capabilities", "submit", "capabilities"} {
		id := string(rune('a' + i))
		if err := encoder.Encode(readRequest{RequestID: id, Command: command}); err != nil {
			t.Fatal(err)
		}
		var event Envelope
		if err := decoder.Decode(&event); err != nil {
			t.Fatal(err)
		}
		if event.RequestID != id || event.Type != "result" || event.OK == (command == "submit") {
			t.Fatalf("%+v", event)
		}
	}
	writer.Close()
	if result := <-done; result != 0 {
		t.Fatal(result)
	}
}
