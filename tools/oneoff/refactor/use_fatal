#!/usr/bin/env ruby

Dir.glob("**/*.go").each do |file|
  next if file == "main.go" || file == "fatal.go" || file.end_with?("_test.go")
  
  content = File.read(file)
  changed = false

  # We look for simple patterns of printing to stderr then os.Exit(1)
  # For example:
  # fmt.Fprintf(os.Stderr, "Error: ...\n", ...)
  # os.Exit(1)
  
  new_content = content.gsub(/fmt\.Fprintf\(os\.Stderr,\s*(".*?")(.*?)\)\n\s*os\.Exit\(1\)/m) do |match|
    changed = true
    "fatalError(#{$1}#{$2})"
  end

  new_content = new_content.gsub(/fmt\.Println\((.*?)\)\n\s*os\.Exit\(1\)/m) do |match|
    changed = true
    "fatalError(\"%s\\n\", #{$1})"
  end
  
  new_content = new_content.gsub(/fmt\.Printf\((".*?")(.*?)\)\n\s*os\.Exit\(1\)/m) do |match|
    changed = true
    "fatalError(#{$1}#{$2})"
  end
  
  if changed
    File.write(file, new_content)
  end
end
