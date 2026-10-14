#!/usr/bin/env ruby

lines = File.readlines("tui.go")
tui_types_lines = File.readlines("tui_types.go")
new_tui_lines = []
move_to_types = []

lines.each_with_index do |line, i|
  idx = i + 1
  # tuiScreen + consts (12-25) and TuiBackend (118-123 original, now it's around line 26-31)
  if line.start_with?("type tuiScreen int") || line.start_with?("const (") || line.start_with?("type TuiBackend struct {") || 
     (idx >= 12 && idx <= 32)
    # Wait, it's safer to just move lines 12 to 32 if they are those exactly.
  end
end
