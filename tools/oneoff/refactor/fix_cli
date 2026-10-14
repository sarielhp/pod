#!/usr/bin/env ruby

code = File.read("cli_parse.go")

# We want to remove the command objects for "opml", "scan", "new", "clean-orphans"
# They look like:
# 			{
# 				Name:        "opml",
#               ...
# 				},
# 			},

def remove_command(code, cmd_name)
  # Find the index of Name:        "cmd_name",
  idx = code.index(%Q{Name:        "#{cmd_name}",})
  return code unless idx

  # Find the starting brace
  start_idx = code.rindex("{", idx)

  # Track braces to find the matching closing brace
  brace_count = 0
  end_idx = start_idx
  while end_idx < code.length
    if code[end_idx] == "{"
      brace_count += 1
    elsif code[end_idx] == "}"
      brace_count -= 1
      if brace_count == 0
        break
      end
    end
    end_idx += 1
  end

  # Also remove the trailing comma and newline
  if code[end_idx + 1] == ","
    end_idx += 1
  end
  while code[end_idx + 1] == " " || code[end_idx + 1] == "\t" || code[end_idx + 1] == "\n"
    end_idx += 1
  end

  # Return the code with this block removed
  code[0...start_idx] + code[(end_idx + 1)..-1]
end

code = remove_command(code, "opml")
code = remove_command(code, "scan")
code = remove_command(code, "new")
code = remove_command(code, "clean-orphans")

norm_func = <<~GO
func normalizeCLIArgs(args []string) []string {
	if len(args) == 0 {
		return args
	}

	firstArg := strings.ToLower(args[0])

	var dummyAct string
	var dummyOpts CLIOptions
	dummyApp := buildCLIApp(&dummyAct, &dummyOpts)
	if isCommandPathOrPrefix(dummyApp, []string{firstArg}) {
		return args
	}

	return args
}
GO

code.gsub!(/func normalizeCLIArgs\(args \[\]string\) \[\]string \{.*?\n\}\n/m, norm_func)

File.write("cli_parse.go", code)
