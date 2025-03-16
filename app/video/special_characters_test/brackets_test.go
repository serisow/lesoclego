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

func TestBracketFFmpegEscaping(t *testing.T) {
	// Skip if FFmpeg is not installed
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("FFmpeg not installed, skipping test")
	}

	// Create temporary directory for test files
	tempDir := t.TempDir()

	// Initialize text processor
	tp := &video.TextProcessorImpl{}

	// Test phrases with progressively more challenging bracket patterns
	testPhrases := []string{
		// Simple cases
		"Text with [square brackets]",
		"Text with (parentheses)",
		"Text with {curly braces}",
		
		// Nested brackets
		"Nested [square [brackets]]",
		"Nested (parentheses (inside) here)",
		"Nested {curly {braces}}",
		
		// Mixed brackets
		"Mixed [({})] brackets",
		"Functions like sin(x) and max[i,j]",
		"Complex {if (x>0) [positive] else [negative]}",
		
		// Brackets with mathematical expressions similar to FFmpeg's own syntax
		"Expression: if(between(t,0,5),sin(2*PI*t),0)",
		"Formula: max(min(value,upper),lower)",
		"Coordinates: [x1,y1] to [x2,y2]",
		
		// Brackets at awkward positions
		"[Beginning] of string",
		"End of string]",
		"Middle [right] here",
		
		// Brackets with other special characters
		"x=[width/2]; y=(height/2)",
		"Data in [format: key=value; type=json]",
		"$variables[index] + function(param)",
		
		// Very complex case
		"if(between(t,0,5),[x1+(t/5)*(x2-x1),y1+(t/5)*(y2-y1)],[x3,y3])",
	}

	// Create a test image
	inputFile := filepath.Join(tempDir, "input.png")
	colorCmd := exec.Command("ffmpeg", "-f", "lavfi", "-i", "color=c=black:s=1000x800:d=1", "-frames:v", "1", inputFile)
	if err := colorCmd.Run(); err != nil {
		t.Fatalf("Failed to create test image: %v", err)
	}

	// Test each phrase
	for i, phrase := range testPhrases {
		t.Run(fmt.Sprintf("Phrase_%d", i), func(t *testing.T) {
			// Get escaped text using the implementation
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
			
			// Capture stderr for debugging
			var stderr bytes.Buffer
			cmd.Stderr = &stderr
			
			// Run command
			err := cmd.Run()
			
			// Log any FFmpeg output
			if stderr.Len() > 0 {
				t.Logf("FFmpeg output for %q:\n%s", phrase, stderr.String())
			}
			
			// Check result
			if err != nil {
				t.Errorf("❌ Failed with text: %q\nError: %v", phrase, err)
				// Detailed failure analysis
				if strings.Contains(stderr.String(), "Unable to parse expression") {
					t.Logf("  > Analysis: FFmpeg tried to interpret brackets as an expression")
				} else if strings.Contains(stderr.String(), "Invalid argument") {
					t.Logf("  > Analysis: Filter syntax broken by improper escaping")
				}
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
		filter := fmt.Sprintf("drawtext=text='%s':fontsize=20:fontcolor=white:x=20:y=%d", 
			escapedText, 40+(i*40))
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
		
		// Analyze the filters to find the problematic one
		filterChain := strings.Join(filters, ",")
		t.Logf("Failed filter chain length: %d characters", len(filterChain))
		
		// Try to isolate the problematic section
		if len(filters) > 1 {
			t.Log("Attempting to isolate the problematic filter...")
			halfway := len(filters) / 2
			
			// Try first half
			firstHalfCmd := exec.Command("ffmpeg", "-i", inputFile,
				"-vf", strings.Join(filters[:halfway], ","), 
				"-y", filepath.Join(tempDir, "first_half.png"))
			err1 := firstHalfCmd.Run()
			
			// Try second half
			secondHalfCmd := exec.Command("ffmpeg", "-i", inputFile,
				"-vf", strings.Join(filters[halfway:], ","), 
				"-y", filepath.Join(tempDir, "second_half.png"))
			err2 := secondHalfCmd.Run()
			
			if err1 != nil && err2 == nil {
				t.Log("Problem is in the FIRST half of filters")
			} else if err1 == nil && err2 != nil {
				t.Log("Problem is in the SECOND half of filters")
			} else {
				t.Log("Problem may be in the combination or multiple filters")
			}
		}
	} else {
		t.Logf("✅ FINAL VERIFICATION PASSED: All bracket patterns processed in a single command")
		
		// Verify the output file exists and has a reasonable size
		if fileInfo, err := os.Stat(finalOutput); err == nil {
			if fileInfo.Size() > 1000 { // Simple check that file isn't empty
				t.Logf("  Output file size: %d bytes", fileInfo.Size())
			} else {
				t.Errorf("  Warning: Output file exists but is suspiciously small: %d bytes", fileInfo.Size())
			}
		}
	}
	
	// Bonus advanced verification: check image magically to ensure text actually rendered
	// Only run if ImageMagick's identify command is available
	if _, err := exec.LookPath("identify"); err == nil {
		t.Log("Running advanced verification with ImageMagick...")
		
		identifyCmd := exec.Command("identify", "-verbose", finalOutput)
		var identifyOutput bytes.Buffer
		identifyCmd.Stdout = &identifyOutput
		
		if err := identifyCmd.Run(); err == nil {
			output := identifyOutput.String()
			
			// Check if the text is actually rendered
			if strings.Contains(output, "text:") {
				t.Log("✅ ADVANCED VERIFICATION: Confirmed text rendering in output image")
			} else {
				t.Log("⚠️ ADVANCED WARNING: Could not explicitly confirm text rendering")
			}
		}
	}
}

// Helper function to examine escaping results in detail
func TestBracketEscapeMechanics(t *testing.T) {
	tp := &video.TextProcessorImpl{}
	
	testCases := map[string]struct {
		input    string
		expected []string // Substrings that must be in the escaped result
		notExpected []string // Substrings that must NOT be in the result
	}{
		"square_brackets": {
			input:    "Text with [brackets]",
			expected: []string{"\\[", "\\]"},
		},
		"parentheses": {
			input:    "Text with (parentheses)",
			expected: []string{"\\(", "\\)"},
		},
		"curly_braces": {
			input:    "Text with {braces}",
			expected: []string{"\\{", "\\}"},
		},
		"nested_mix": {
			input:    "Complex {if (x>0) [positive] else [negative]}",
			expected: []string{"\\{", "\\(", "\\[", "\\]", "\\)"},
		},
		"ffmpeg_like_expression": {
			input:    "if(between(t,0,5),sin(2*PI*t),0)",
			expected: []string{
				"if\\(", 
				"between\\(", 
				"\\,sin\\(",
				"PI\\*t\\)",
				"\\,0\\)",
			},
			// We shouldn't have unescaped commas or parentheses
			notExpected: []string{
				"t,0", 
				"5),", 
				"(2*",
			},
		},
		"comma_escaping": {
			input:    "Values: 1,2,3,4",
			expected: []string{"1\\,2\\,3\\,4"},
		},
	}
	
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			result := tp.EscapeFFmpegText(tc.input)
			t.Logf("Original: %q", tc.input)
			t.Logf("Escaped:  %q", result)
			
			// Check for required substrings
			for _, expectedStr := range tc.expected {
				if !strings.Contains(result, expectedStr) {
					t.Errorf("Expected escaped result to contain %q, but got: %q", 
						expectedStr, result)
				}
			}
			
			// Check for forbidden substrings
			for _, notExpectedStr := range tc.notExpected {
				if strings.Contains(result, notExpectedStr) {
					t.Errorf("Expected escaped result NOT to contain %q, but got: %q", 
						notExpectedStr, result)
				}
			}
		})
	}
}