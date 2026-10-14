#!/usr/bin/env ruby
# frozen_string_literal: true

root_dir = File.expand_path('../..', __dir__)

# 1. Update README.md
readme_path = File.join(root_dir, 'README.md')
src = File.read(readme_path)
src.sub!(/\A# abs\b/, '# pod')
src.gsub!(/`abs`/i, '`pod`')
src.gsub!(/`~\/\.config\/abs\/config\.json`/, '`~/.config/pod/config.json` (with fallback to `~/.config/abs/config.json`)')
src.gsub!(/`~\/\.config\/abs\/podcasts\.json`/, '`~/.config/pod/podcasts.json` (or `~/.config/abs/podcasts.json`)')
src.gsub!(/--input-ipc-server=\/tmp\/abs_player\.sock/, '--input-ipc-server=/tmp/pod_player.sock')
src.gsub!(/\/tmp\/abs_player\.sock/, '/tmp/pod_player.sock')
src.gsub!(/go build -o abs \./, 'go build -o pod . # (or ./tools/build_local to build ./pod and ./abs symlink)')
# Replace command lines
src.gsub!(/^(abs\s+)/, 'pod ')
src.gsub!(/`abs\s+([^`]+)`/, '`pod \1`')
src.gsub!(/## Background Audio Player \(`abs player`\)/, '## Background Audio Player (`pod player`)')
File.write(readme_path, src)
puts "Updated README.md"

# 2. Update antenna_pod.md
ap_path = File.join(root_dir, 'antenna_pod.md')
src = File.read(ap_path)
src.gsub!(/managed by `abs`/, 'managed by `pod`')
src.gsub!(/ABS Feeds/, 'Pod Feeds')
src.gsub!(/`abs server feeds`/, '`pod server feeds`')
src.gsub!(/`abs server feed`/, '`pod server feed`')
File.write(ap_path, src)
puts "Updated antenna_pod.md"

# 3. Update architecture.md
arch_path = File.join(root_dir, 'architecture.md')
src = File.read(arch_path)
src.sub!(/# Architecture of `abs`/, '# Architecture of `pod`')
src.gsub!(/`abs`/i, '`pod`')
src.gsub!(/~\/\.config\/abs\/podcasts\.json/, '~/.config/pod/podcasts.json')
src.gsub!(/~\/\.config\/abs\/config\.json/, '~/.config/pod/config.json')
src.gsub!(/abs native downloader/, 'pod native downloader')
src.gsub!(/abs server import/, 'pod server import')
File.write(arch_path, src)
puts "Updated architecture.md"

# 4. Update AGENTS.md
agents_path = File.join(root_dir, 'AGENTS.md')
src = File.read(agents_path)
src.gsub!(/`tools\/build_local` \| Build local `\.\/abs` binary/, '`tools/build_local` | Build local `./pod` binary (with `./abs` symlink)')
src.gsub!(/`~\/\.config\/abs\/podcasts\.json`/, '`~/.config/pod/podcasts.json`')
src.gsub!(/`~\/\.config\/abs\/config\.json`/, '`~/.config/pod/config.json`')
src.gsub!(/\/tmp\/abs_player\.sock/, '/tmp/pod_player.sock')
src.gsub!(/`abs player/, '`pod player')
src.gsub!(/`abs config migrate/, '`pod config migrate')
src.gsub!(/`abs server import/, '`pod server import')
src.gsub!(/`abs` is its own native podcast backend/, '`pod` is its own native podcast backend')
File.write(agents_path, src)
puts "Updated AGENTS.md"
