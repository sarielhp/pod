#!/usr/bin/env ruby

Dir.glob("**/*.go").each do |file|
  next if file == "main.go" || file == "fatal.go" || file.end_with?("_test.go")
  
  content = File.read(file)
  changed = false

  # printError(...) -> fatalError("%s\n", ...)
  new_content = content.gsub(/printError\((.*?)\)\n\s*os\.Exit\(1\)/) do |match|
    changed = true
    "fatalError(\"%s\\n\", #{$1})"
  end

  # fmt.Fprintf(os.Stderr, ...) -> fatalError(...)
  new_content = new_content.gsub(/fmt\.Fprintf\(os\.Stderr,\s*(.*?)\)\n\s*os\.Exit\(1\)/) do |match|
    changed = true
    "fatalError(#{$1})"
  end

  # fmt.Println(...) -> fatalError("%s\n", ...)
  new_content = new_content.gsub(/fmt\.Println\((.*?)\)\n\s*os\.Exit\(1\)/) do |match|
    changed = true
    "fatalError(\"%s\\n\", #{$1})"
  end
  
  if changed
    File.write(file, new_content)
  end
end
