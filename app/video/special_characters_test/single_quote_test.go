package special_characters_test

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/serisow/lesocle/video"
)

func TestSingleQuoteFFmpegEscaping(t *testing.T) {
	// Skip if FFmpeg is not installed
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("FFmpeg not installed, skipping test")
	}

	// Create temporary directory for test files
	tempDir := t.TempDir()

	// Initialize text processor
	tp := &video.TextProcessorImpl{}

	// Test phrases with progressively more challenging single quotes
	testPhrases := []string{
		"It's working",
		"Don't say 'stop'",
		"John's book: 'War and Peace'",
		"Multiple 'single' 'quotes' 'everywhere'",
		"L'hôtel est à l'ouest de la ville",
		"Alternating 'single' and \"double\" quotes",
		"Quote at start: 'This is important'",
		"The most challenging: It's John's dog's toy's 'color'",
	}

	// Create a test image
	inputFile := filepath.Join(tempDir, "input.png")
	colorCmd := exec.Command("ffmpeg", "-f", "lavfi", "-i", "color=c=black:s=800x600:d=1", "-frames:v", "1", inputFile)
	if err := colorCmd.Run(); err != nil {
		t.Fatalf("Failed to create test image: %v", err)
	}

	// Test each phrase
	for i, phrase := range testPhrases {
		t.Run(fmt.Sprintf("Phrase_%d", i), func(t *testing.T) {
			// Get escaped text using current implementation
			escapedText := tp.EscapeFFmpegText(phrase)
			
			// Output file for this test
			outputFile := filepath.Join(tempDir, fmt.Sprintf("output_%d.png", i))
			
			// Build drawtext filter with the escaped text
			filter := fmt.Sprintf("drawtext=text='%s':fontsize=24:fontcolor=white:x=20:y=%d", 
				escapedText, 50+(i*40))
			
			// Log test details for debugging
			t.Logf("Original: %q", phrase)
			t.Logf("Escaped:  %q", escapedText)
			t.Logf("Filter:   %s", filter)
			
			// Create FFmpeg command
			cmd := exec.Command("ffmpeg", "-i", inputFile, "-vf", filter, "-y", outputFile)
			
			// Capture output
			var stderr bytes.Buffer
			cmd.Stderr = &stderr
			
			// Run command
			err := cmd.Run()
			
			// Log any FFmpeg output
			if stderr.Len() > 0 {
				t.Logf("FFmpeg output:\n%s", stderr.String())
			}
			
			// Check result
			if err != nil {
				t.Errorf("❌ Failed with text: %q\nError: %v", phrase, err)
			} else if _, err := os.Stat(outputFile); os.IsNotExist(err) {
				t.Errorf("❌ Output file not created for: %q", phrase)
			} else {
				t.Logf("✅ Successfully processed: %q", phrase)
			}
		})
	}
	
	// Final verification: create single image with all test phrases
	// This proves all phrases can work simultaneously in one command
	var filters []string
	for i, phrase := range testPhrases {
		escapedText := tp.EscapeFFmpegText(phrase)
		filter := fmt.Sprintf("drawtext=text='%s':fontsize=24:fontcolor=white:x=20:y=%d", 
			escapedText, 50+(i*40))
		filters = append(filters, filter)
	}
	
	finalOutput := filepath.Join(tempDir, "final_verification.png")
	finalCmd := exec.Command("ffmpeg", "-i", inputFile, 
		"-vf", strings.Join(filters, ","), "-y", finalOutput)
	
	var finalStderr bytes.Buffer
	finalCmd.Stderr = &finalStderr
	
	if err := finalCmd.Run(); err != nil {
		t.Errorf("❌ Final verification failed: %v\n%s", 
			err, finalStderr.String())
	} else {
		t.Logf("✅ FINAL VERIFICATION PASSED: All phrases processed in a single command")
	}
}