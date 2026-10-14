#!/usr/bin/env ruby
# frozen_string_literal: true

root_dir = File.expand_path('../..', __dir__)
cli_dir = File.join(root_dir, 'pkg', 'cli')

# 1. Update app.go
app_go = File.join(cli_dir, 'app.go')
src = File.read(app_go)
src.sub!(/Name:\s*"abs",/, 'Name:                "pod",')
src.sub!(/UsageLine:\s*"abs \[OPTIONS\] <COMMAND>",/, 'UsageLine:           "pod [OPTIONS] <COMMAND>",')
src.sub!(/Run 'abs /, "Run 'pod ")
File.write(app_go, src)
puts "Updated app.go"

# 2. Update parse.go
parse_go = File.join(cli_dir, 'parse.go')
src = File.read(parse_go)
src.gsub!(/name := "abs"/, 'name := "pod"')
src.gsub!(/name = "abs " \+ /, 'name = "pod " + ')
File.write(parse_go, src)
puts "Updated parse.go"

# 3. Update tests
parse_test = File.join(cli_dir, 'parse_test.go')
src = File.read(parse_test)
src.gsub!('"abs"', '"pod"')
File.write(parse_test, src)
puts "Updated parse_test.go"

cache_test = File.join(cli_dir, 'config_cache_test.go')
src = File.read(cache_test)
src.gsub!('"abs"', '"pod"')
File.write(cache_test, src)
puts "Updated config_cache_test.go"

freq_test = File.join(cli_dir, 'server_frequency_test.go')
src = File.read(freq_test)
src.gsub!('unknown command "block" for "abs"', 'unknown command "block" for "pod"')
File.write(freq_test, src)
puts "Updated server_frequency_test.go"

# 4. Update all files in pkg/cli
files = Dir.glob(File.join(cli_dir, '*.go'))
count = 0
files.each do |file|
  s = File.read(file)
  orig = s.dup
  # UsageLine and Line
  s.gsub!(/(UsageLine:\s*)"abs /, "\\1\"pod ")
  s.gsub!(/(Line:\s*)"abs /, "\\1\"pod ")
  s.gsub!(/\"abs server /, "\"pod server ")
  s.gsub!(/\"abs queue /, "\"pod queue ")
  s.gsub!(/\"abs config /, "\"pod config ")
  s.gsub!(/\"abs offload /, "\"pod offload ")
  s.gsub!(/\"abs rm_ads /, "\"pod rm_ads ")
  s.gsub!(/\"abs player /, "\"pod player ")
  s.gsub!(/\"abs info /, "\"pod info ")
  s.gsub!(/\"abs tui /, "\"pod tui ")
  s.gsub!(/\bRun 'abs /, "Run 'pod ")
  s.gsub!(/\bUse 'abs /, "Use 'pod ")
  s.gsub!(/\bfor 'abs /, "for 'pod ")
  s.gsub!(/'abs server /, "'pod server ")
  s.gsub!(/'abs queue /, "'pod queue ")
  s.gsub!(/'abs config /, "'pod config ")
  s.gsub!(/'abs offload /, "'pod offload ")
  s.gsub!(/'abs rm_ads /, "'pod rm_ads ")
  s.gsub!(/'abs player /, "'pod player ")
  s.gsub!(/'abs info /, "'pod info ")
  s.gsub!(/'abs tui /, "'pod tui ")
  s.gsub!(/fmt\.Println\("Usage: abs /, 'fmt.Println("Usage: pod ')
  s.gsub!(/\(defaults to ~\/abs_remote\)/, '(defaults to ~/abs_remote or ~/pod_remote)')
  if s != orig
    File.write(file, s)
    count += 1
  end
end
puts "Updated CLI usage strings across #{count} files in pkg/cli."
