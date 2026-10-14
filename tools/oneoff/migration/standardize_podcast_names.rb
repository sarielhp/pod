#!/usr/bin/env ruby
# frozen_string_literal: true

require "fileutils"
require "json"
require "optparse"
require "rexml/document"
require "time"

options = {
  execute: false,
  base_dir: "/media/podcasts/clean",
  config_dirs: [
    File.expand_path("~/.config/abs"),
    File.expand_path("~/.config/pod")
  ]
}

OptionParser.new do |opts|
  opts.banner = "Usage: standardize_podcast_names.rb [options]"
  opts.on("-e", "--execute", "Actually perform the renames and updates (default: dry run)") do
    options[:execute] = true
  end
  opts.on("-d", "--dir DIR", "Base podcasts directory") do |d|
    options[:base_dir] = File.expand_path(d)
  end
end.parse!

def sanitize_slug(str)
  s = str.to_s.strip
  return "untitled" if s.empty?

  # Remove quotes and apostrophes completely
  s = s.gsub(/[\x27\x22\u2018\u2019\u201C\u201D`]/, "")
  # Non-alphanumeric unicode runes become underscores
  s = s.gsub(/[^\p{L}\p{N}]+/, "_")
  s = s.gsub(/_+/, "_")
  s = s.gsub(/\A_+|_\z/, "")
  return "untitled" if s.empty? || s == "." || s == ".."

  # Truncate to 120 runes max
  runes = s.chars
  if runes.length > 120
    s = runes[0...120].join.gsub(/_+\z/, "")
  end
  s.empty? ? "untitled" : s
end

def format_episode_filename(pub_date, ep_num, title)
  clean_title = sanitize_slug(title)
  parts = []

  if pub_date && pub_date.is_a?(Time)
    parts << pub_date.strftime("%Y-%m-%d")
  end

  clean_ep = ep_num.to_s.strip
  if !clean_ep.empty? && clean_ep != "0"
    digits = clean_ep.gsub(/[^\d]/, "")
    if !digits.empty?
      ep_tag = "ep#{digits}"
      lower_title = clean_title.downcase
      if !lower_title.start_with?("#{ep_tag.downcase}_") &&
         !lower_title.start_with?("ep") &&
         !lower_title.start_with?("#{digits}_")
        parts << ep_tag
      end
    end
  end

  parts << clean_title
  "#{parts.join("_")}.mp3"
end

def parse_feed_xml(feed_path)
  return {} unless File.exist?(feed_path)

  map = {}
  begin
    doc = REXML::Document.new(File.read(feed_path))
    doc.elements.each("//item") do |item|
      title = item.elements["title"]&.text&.strip
      pub_str = item.elements["pubDate"]&.text&.strip
      ep_num = item.elements["itunes:episode"]&.text&.strip
      t = nil
      begin
        t = Time.parse(pub_str).utc if pub_str
      rescue StandardError
        # ignore parse error
      end
      data = { time: t, ep: ep_num, title: title }
      if title
        map[title.downcase] = data
        map[sanitize_slug(title).downcase] = data
      end
      enc_url = item.elements["enclosure"]&.attributes&.[]("url")
      if enc_url
        u_base = File.basename(enc_url).sub(/\.mp3\z/i, "").downcase
        map[u_base] = data
      end
    end
  rescue StandardError => e
    warn "Warning: failed to parse #{feed_path}: #{e}"
  end
  map
end

def episode_is_cleaned?(mp3_path)
  base = mp3_path.sub(/\.mp3\z/i, "")
  return true if File.exist?("#{base}.cuts.json")

  status_file = "#{mp3_path}.json"
  if File.exist?(status_file)
    begin
      data = JSON.parse(File.read(status_file))
      status = data["status"].to_s.downcase
      return true if %w[ad_removal_completed clean completed].include?(status)
      return true if data["cleaned"] && data["cleaned"]["filename"]
    rescue StandardError
      # ignore
    end
  end
  false
end

clean_dir = options[:base_dir]
execute = options[:execute]

puts "=== Podcast Standardization #{execute ? '(LIVE EXECUTION)' : '(DRY RUN)'} ==="
puts "Podcast root: #{clean_dir}"

# Ensure ~/.config/pod exists and is synced with ~/.config/abs
abs_dir = File.expand_path("~/.config/abs")
pod_dir = File.expand_path("~/.config/pod")
if File.exist?(abs_dir) && !File.exist?(pod_dir) && execute
  FileUtils.mkdir_p(pod_dir)
  FileUtils.cp_r(File.join(abs_dir, "."), pod_dir)
  puts "Initialized #{pod_dir} from #{abs_dir}"
end

# Step 1: Discover Show Directories & Compute Mappings
show_dirs = Dir.children(clean_dir).map { |c| File.join(clean_dir, c) }.select { |p| File.directory?(p) }
dir_targets = {}
show_dirs.each do |dir|
  name = File.basename(dir)
  target_name = sanitize_slug(name)
  target_dir = File.join(clean_dir, target_name)
  dir_targets[dir] = target_dir
end

# Check show directory consolidations
dir_groups = dir_targets.group_by { |_src, dst| dst }
consolidations = dir_groups.select { |_dst, srcs| srcs.length > 1 }

puts "\nFound #{show_dirs.length} show directories."
if consolidations.any?
  puts "Directory Consolidations (#{consolidations.length}):"
  consolidations.each do |target_dir, srcs|
    puts "  Target: #{File.basename(target_dir)}"
    srcs.each { |src, _| puts "    <- #{File.basename(src)}" }
  end
end

# Step 2: Merge duplicate directories if executing
if execute
  consolidations.each do |target_dir, srcs|
    FileUtils.mkdir_p(target_dir)
    srcs.each do |src_dir, _|
      next if src_dir == target_dir

      # Move files from src_dir into target_dir
      Dir.children(src_dir).each do |child|
        src_child = File.join(src_dir, child)
        dst_child = File.join(target_dir, child)
        if File.exist?(dst_child)
          if File.size(src_child) == File.size(dst_child)
            FileUtils.rm_f(src_child)
          else
            FileUtils.mv(src_child, File.join(target_dir, "alt_#{child}"))
          end
        else
          FileUtils.mv(src_child, dst_child)
        end
      end
      FileUtils.rmdir(src_dir) if Dir.empty?(src_dir)
    end
  end

  # Rename remaining single directories if src != dst
  dir_groups.each do |target_dir, srcs|
    next if srcs.length > 1

    src_dir = srcs.first.first
    if src_dir != target_dir && File.exist?(src_dir)
      FileUtils.mv(src_dir, target_dir)
    end
  end
end

# Re-read show directories post-merge
current_show_dirs = Dir.children(clean_dir).map { |c| File.join(clean_dir, c) }.select { |p| File.directory?(p) }

# Step 3: Process Episodes inside each directory
total_mp3s = 0
total_renames = 0
total_duplicates_merged = 0
companion_renames = 0
status_updates = 0

current_show_dirs.each do |show_dir|
  feed_map = parse_feed_xml(File.join(show_dir, "feed.xml"))
  mp3s = Dir.glob("#{show_dir}/*.mp3")
  total_mp3s += mp3s.length

  # Group target filenames within show to detect collisions
  target_plan = Hash.new { |h, k| h[k] = [] }

  mp3s.each do |mp3|
    base = File.basename(mp3, ".mp3")
    clean_stem = sanitize_slug(base)

    # Resolve date and episode number
    feed_data = feed_map[base.downcase] || feed_map[clean_stem.downcase]
    pub_date = feed_data ? feed_data[:time] : nil
    ep_num = feed_data ? feed_data[:ep] : nil

    # Fallback to status json
    status_file = "#{mp3}.json"
    if File.exist?(status_file)
      begin
        st = JSON.parse(File.read(status_file))
        if !pub_date && st["published_at"]
          pub_date = Time.parse(st["published_at"]).utc rescue nil
        end
      rescue StandardError
        # ignore
      end
    end

    # Fallback to mtime
    pub_date ||= File.mtime(mp3).utc

    # Fallback episode number extraction from title regex
    if !ep_num || ep_num.empty?
      if base =~ /\b(?:ep|episode|פרק)[\s._#-]*(\d+)\b/i
        ep_num = ::Regexp.last_match(1)
      end
    end

    target_fn = format_episode_filename(pub_date, ep_num, base)
    target_plan[target_fn] << mp3
  end

  # Execute planned renames for this show
  target_plan.each do |target_fn, source_files|
    target_mp3 = File.join(show_dir, target_fn)

    # Select canonical file if multiple source files mapped to this target
    chosen_source = nil
    if source_files.length == 1
      chosen_source = source_files.first
    else
      # Sort by: 1) is_cleaned?, 2) size, 3) mtime
      sorted = source_files.sort_by do |f|
        cleaned_score = episode_is_cleaned?(f) ? 10 : 0
        [cleaned_score, File.size(f)]
      end
      chosen_source = sorted.last
      duplicates = source_files - [chosen_source]

      puts "  [MERGE DUPLICATE] Show: #{File.basename(show_dir)}"
      puts "    Chosen: #{File.basename(chosen_source)} (cleaned: #{episode_is_cleaned?(chosen_source)})"
      duplicates.each do |dup|
        puts "    Retired dup: #{File.basename(dup)}"
        total_duplicates_merged += 1
        if execute
          # Remove duplicate mp3 and companion files safely without glob issues
          dup_base = File.basename(dup, ".mp3")
          Dir.children(show_dir).each do |child|
            if child == "#{dup_base}.mp3" || child.start_with?("#{dup_base}.")
              FileUtils.rm_f(File.join(show_dir, child))
            end
          end
        end
      end
    end

    next unless chosen_source

    if File.basename(chosen_source) != target_fn
      total_renames += 1
      src_base = File.basename(chosen_source, ".mp3")
      dst_base = File.basename(target_mp3, ".mp3")

      if execute
        # Rename companion files safely using string prefix
        Dir.children(show_dir).each do |child|
          next if child == "#{src_base}.mp3" # handle mp3 explicitly
          if child.start_with?("#{src_base}.")
            suffix = child[src_base.length..-1]
            src_file = File.join(show_dir, child)
            dst_file = File.join(show_dir, "#{dst_base}#{suffix}")
            FileUtils.mv(src_file, dst_file)
            companion_renames += 1
          end
        end

        # Rename audio file
        FileUtils.mv(chosen_source, target_mp3)

        # Update .mp3.json metadata
        status_file = File.join(show_dir, "#{dst_base}.mp3.json")
        if File.exist?(status_file)
          begin
            data = JSON.parse(File.read(status_file))
            data["media_file"] = target_fn
            data["original"]["filename"] = target_fn if data["original"]
            data["cleaned"]["filename"] = target_fn if data["cleaned"]
            File.write(status_file, JSON.pretty_generate(data))
            status_updates += 1
          rescue StandardError => e
            warn "Error updating #{status_file}: #{e}"
          end
        end
      end
    end
  end
end

# Step 4: Clean up stale locks
locks = Dir.glob("#{clean_dir}/**/*.lock")
if execute && locks.any?
  locks.each { |l| FileUtils.rm_f(l) }
  puts "\nRemoved #{locks.length} stale lock files."
end

# Step 5: Update podcasts.json subscriptions
options[:config_dirs].each do |cfg_dir|
  sub_file = File.join(cfg_dir, "podcasts.json")
  next unless File.exist?(sub_file)

  begin
    cfg = JSON.parse(File.read(sub_file))
    subs = cfg["subscriptions"] || []
    updated_subs = 0
    subs.each do |sub|
      old_folder = sub["folder"]
      new_folder = sanitize_slug(sub["title"])
      if old_folder != new_folder
        sub["folder"] = new_folder
        updated_subs += 1
      end
    end
    puts "\nSubscriptions in #{sub_file}: #{subs.length} total, #{updated_subs} folder updates planned."
    if execute
      bak_file = "#{sub_file}.bak.#{Time.now.to_i}"
      FileUtils.cp(sub_file, bak_file)
      File.write(sub_file, JSON.pretty_generate(cfg))
      puts "  Updated #{sub_file} (backup saved to #{bak_file})."
    end
  rescue StandardError => e
    warn "Error updating #{sub_file}: #{e}"
  end
end

puts "\n=== Migration Summary ==="
puts "Total MP3 files inspected: #{total_mp3s}"
puts "Episodes to rename: #{total_renames}"
puts "Duplicate episode copies to merge/retire: #{total_duplicates_merged}"
puts "Companion files moved: #{companion_renames}" if execute
puts "Status JSON files updated: #{status_updates}" if execute
puts "\nDone. #{execute ? 'Migration COMPLETE!' : 'Run with --execute to apply changes.'}"
