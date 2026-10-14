#!/usr/bin/env ruby
# frozen_string_literal: true

require "fileutils"
require "json"
require "net/http"
require "open3"
require "optparse"
require "rexml/document"
require "shellwords"
require "time"

options = {
  execute: false,
  base_dir: "/media/podcasts/clean",
  podcasts_file: File.expand_path("~/.config/pod/podcasts.json"),
  target_podcast: nil,
  verbose: false
}

OptionParser.new do |opts|
  opts.banner = "Usage: repair_episode_dates.rb [options]"
  opts.on("-e", "--execute", "Actually perform renames and status updates (default: dry-run)") do
    options[:execute] = true
  end
  opts.on("-d", "--dir DIR", "Base podcasts directory") do |d|
    options[:base_dir] = File.expand_path(d)
  end
  opts.on("-p", "--podcast QUERY", "Target specific podcast by folder or title substring") do |p|
    options[:target_podcast] = p
  end
  opts.on("-v", "--verbose", "Show detailed debug information") do
    options[:verbose] = true
  end
end.parse!

def sanitize_slug(str)
  s = str.to_s.strip
  return "untitled" if s.empty?

  s = s.gsub(/[\x27\x22\u2018\u2019\u201C\u201D`]/, "")
  s = s.gsub(/[^\p{L}\p{N}]+/, "_")
  s = s.gsub(/_+/, "_")
  s = s.gsub(/\A_+|_\z/, "")
  return "untitled" if s.empty? || s == "." || s == ".."

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

def normalize_feed_tz(str)
  tz_map = {
    "PDT" => "-0700", "PST" => "-0800",
    "EDT" => "-0400", "EST" => "-0500",
    "CDT" => "-0500", "CST" => "-0600",
    "MDT" => "-0600", "MST" => "-0700",
    "UTC" => "+0000", "GMT" => "+0000", "UT" => "+0000", "Z" => "+0000",
    "BST" => "+0100", "WEST" => "+0100", "WET" => "+0000",
    "CEST" => "+0200", "CET" => "+0100",
    "EEST" => "+0300", "EET" => "+0200",
    "IDT" => "+0300", "IST" => "+0200", "MSK" => "+0300",
    "JST" => "+0900", "KST" => "+0900",
    "AEST" => "+1000", "AEDT" => "+1100"
  }
  s = str.strip
  tz_map.each do |k, v|
    if s.end_with?(" #{k}") || s.end_with?("\t#{k}")
      return s[0...-k.length].strip + " " + v
    end
  end
  s
end

def parse_pub_time(pub_str)
  return nil if pub_str.nil? || pub_str.strip.empty?
  norm = normalize_feed_tz(pub_str)
  Time.parse(norm).utc
rescue StandardError
  nil
end

def fetch_feed_items(feed_url)
  return [] if feed_url.nil? || feed_url.strip.empty?

  uri = URI(feed_url)
  http = Net::HTTP.new(uri.host, uri.port)
  http.use_ssl = (uri.scheme == "https")
  http.open_timeout = 10
  http.read_timeout = 15

  req = Net::HTTP::Get.new(uri.request_uri)
  req["User-Agent"] = "Mozilla/5.0 (compatible; pod/0.2.77)"
  res = http.request(req)

  if res.code =~ /30[12378]/ && res["location"]
    new_uri = URI(res["location"])
    http = Net::HTTP.new(new_uri.host, new_uri.port)
    http.use_ssl = (new_uri.scheme == "https")
    http.open_timeout = 10
    http.read_timeout = 15
    res = http.request(Net::HTTP::Get.new(new_uri.request_uri))
  end

  return [] unless res.code == "200"

  items = []
  doc = REXML::Document.new(res.body)
  doc.elements.each("//item") do |it|
    title = it.elements["title"]&.text&.strip
    pub = it.elements["pubDate"]&.text&.strip
    enc = it.elements["enclosure"]&.attributes&.[]("url")
    guid = it.elements["guid"]&.text&.strip
    ep_num = it.elements["itunes:episode"]&.text&.strip

    t = parse_pub_time(pub)
    items << {
      title: title,
      pub_raw: pub,
      time: t,
      enclosure: enc,
      guid: guid,
      ep_num: ep_num
    }
  end
  items
rescue StandardError => e
  warn "Failed to fetch/parse feed #{feed_url}: #{e.message}"
  []
end

def probe_id3_date(mp3_path)
  out, status = Open3.capture2("ffprobe", "-v", "error", "-show_entries", "format_tags=date", "-of", "default=noprint_wrappers=1:nokey=1", mp3_path)
  return nil unless status.success?
  out = out.strip
  return nil if out.empty?
  parse_pub_time(out)
rescue StandardError
  nil
end

def probe_id3_title(mp3_path)
  out, status = Open3.capture2("ffprobe", "-v", "error", "-show_entries", "format_tags=title", "-of", "default=noprint_wrappers=1:nokey=1", mp3_path)
  return nil unless status.success?
  out = out.strip
  out.empty? ? nil : out
rescue StandardError
  nil
end

def parse_embedded_title_date(mp3_path)
  base = File.basename(mp3_path, ".mp3")
  if base =~ /(?:^|[^\d])(0[1-9]|1[0-2])[-_](0[1-9]|[12]\d|3[01])[-_](20\d\d)(?:$|[^\d])/
    m, d, y = Regexp.last_match(1), Regexp.last_match(2), Regexp.last_match(3)
    hour = 12
    if base =~ /(?:^|[^\d])([1-9]|1[0-2])(AM|PM)/i
      h_val = Regexp.last_match(1).to_i
      meridiem = Regexp.last_match(2).upcase
      h_val = 0 if meridiem == "AM" && h_val == 12
      h_val += 12 if meridiem == "PM" && h_val < 12
      hour = h_val
    end
    return Time.utc(y.to_i, m.to_i, d.to_i, hour, 0, 0)
  end
  nil
rescue StandardError
  nil
end

def match_file_to_feed(mp3_path, feed_items)
  base = File.basename(mp3_path, ".mp3")
  clean_stem = sanitize_slug(base)

  # Check companion json for guid or enclosure
  json_path = "#{mp3_path}.json"
  if File.exist?(json_path)
    begin
      data = JSON.parse(File.read(json_path))
      guid = data["guid"] || data["id"]
      if guid && !guid.empty?
        m = feed_items.find { |fi| fi[:guid] == guid }
        return m if m
      end
    rescue StandardError
      # ignore
    end
  end

  # Check episode number in filename (e.g. ep238 or פרק 238)
  if base =~ /\b(?:ep|episode|פרק)[\s._#-]*(\d+)\b/i
    num = Regexp.last_match(1)
    m = feed_items.find do |fi|
      fi[:ep_num] == num ||
        fi[:title] =~ /\b(?:ep|episode|פרק)[\s._#-]*#{num}\b/i
    end
    return m if m
  end

  # Strip date prefix if present
  stripped = base.sub(/^\d{4}-\d{2}-\d{2}_/, "")
  clean_stripped = sanitize_slug(stripped)

  # Exact title match
  m = feed_items.find do |fi|
    fi_title = fi[:title].to_s
    fi_clean = sanitize_slug(fi_title)
    fi_clean.downcase == clean_stem.downcase ||
      fi_clean.downcase == clean_stripped.downcase ||
      fi_title.downcase == base.downcase ||
      fi_title.downcase == stripped.downcase
  end
  return m if m

  # Substring or word match
  file_date = parse_embedded_title_date(mp3_path)
  words = clean_stripped.split("_").select { |w| w.length >= 3 }
  if words.length >= 3
    candidates = feed_items.select do |fi|
      fi_clean = sanitize_slug(fi[:title].to_s).downcase
      if file_date && fi[:time]
        next false if fi[:time].strftime("%Y-%m-%d") != file_date.strftime("%Y-%m-%d")
      end
      matched_words = words.count { |w| fi_clean.include?(w.downcase) }
      matched_words.to_f / words.length >= 0.75
    end
    return candidates.first if candidates.length == 1
  end

  # Enclosure URL base match
  feed_items.find do |fi|
    next unless fi[:enclosure]
    enc_base = File.basename(fi[:enclosure]).sub(/\.mp3\z/i, "").downcase
    enc_base == base.downcase || enc_base == clean_stem.downcase
  end
end

clean_dir = options[:base_dir]
execute = options[:execute]

puts "=== Podcast Episode Date Repair #{execute ? '(LIVE EXECUTION)' : '(DRY RUN)'} ==="
puts "Library: #{clean_dir}"

subs_file = options[:podcasts_file]
unless File.exist?(subs_file)
  subs_file = File.expand_path("~/.config/abs/podcasts.json")
end
abort "Error: Subscriptions file not found at #{subs_file}" unless File.exist?(subs_file)

subs = JSON.parse(File.read(subs_file))["subscriptions"]
puts "Loaded #{subs.length} podcast subscriptions."

total_repaired = 0
total_renamed = 0
total_duplicates_removed = 0

subs.each do |sub|
  folder = sub["folder"] || sub["title"]
  folder_name = sanitize_slug(folder)
  dir = File.join(clean_dir, folder_name)
  dir = File.join(clean_dir, folder) unless File.directory?(dir)
  next unless File.directory?(dir)

  if options[:target_podcast]
    q = options[:target_podcast].downcase
    next unless folder_name.downcase.include?(q) || sub["title"].downcase.include?(q)
  end

  mp3s = Dir.glob("#{dir}/*.mp3")
  next if mp3s.empty?

  puts "\nProcessing show: #{sub["title"]} (#{mp3s.length} episodes)"
  feed_items = fetch_feed_items(sub["feed_url"])
  if feed_items.empty?
    puts "  [WARN] No feed items retrieved for #{sub["feed_url"]}"
    next
  end

  # Build a target mapping by target_fn to detect duplicates and planned renames
  target_map = Hash.new { |h, k| h[k] = [] }
  meta_map = {}

  mp3s.each do |mp3|
    matched = match_file_to_feed(mp3, feed_items)
    if matched && matched[:time]
      pub_time = matched[:time]
      ep_num = matched[:ep_num]
      ep_title = matched[:title]
      target_fn = format_episode_filename(pub_time, ep_num, ep_title)
      target_map[target_fn] << mp3
      meta_map[target_fn] = matched
    else
      id3_date = probe_id3_date(mp3) || parse_embedded_title_date(mp3)
      if id3_date
        id3_title = probe_id3_title(mp3)
        base_no_ext = File.basename(mp3, ".mp3")
        stripped = base_no_ext.sub(/^\d{4}-\d{2}-\d{2}_/, "")
        ep_title = (id3_title && !id3_title.strip.empty?) ? id3_title : stripped
        target_fn = format_episode_filename(id3_date, nil, ep_title)
        matched = { title: ep_title, time: id3_date, enclosure: nil, guid: nil, ep_num: nil }
        target_map[target_fn] << mp3
        meta_map[target_fn] = matched
      end
    end
  end

  target_map.each do |target_fn, files|
    matched = meta_map[target_fn]
    next unless matched && matched[:time]

    pub_time = matched[:time]
    target_mp3 = File.join(dir, target_fn)

    # Handle duplicates: multiple local files matching the same feed item
    chosen_file = nil
    if files.length == 1
      chosen_file = files.first
    else
      # Sort: prefer cuts, then transcripts, then larger size
      sorted = files.sort_by do |f|
        has_cuts = File.exist?("#{f.sub(/\.mp3\z/i, "")}.cuts.json") ? 10 : 0
        has_tx = File.exist?("#{f.sub(/\.mp3\z/i, "")}.transcript.json") ? 5 : 0
        sz = File.exist?(f) ? File.size(f) : 0
        [has_cuts + has_tx, sz]
      end
      chosen_file = sorted.last
      duplicates = files - [chosen_file]

      duplicates.each do |dup|
        dup_base = dup.sub(/\.mp3\z/i, "")
        chosen_base = chosen_file.sub(/\.mp3\z/i, "")
        [".cuts.json", ".transcript.json", ".srt", ".txt", ".precut"].each do |ext|
          src_f = "#{dup_base}#{ext}"
          dst_f = "#{chosen_base}#{ext}"
          if File.exist?(src_f) && !File.exist?(dst_f) && execute
            FileUtils.cp(src_f, dst_f)
          end
        end

        puts "  [RETIRE DUPLICATE] #{File.basename(dup)} (keeping #{File.basename(chosen_file)})"
        total_duplicates_removed += 1
        if execute
          dup_child_base = File.basename(dup, ".mp3")
          Dir.children(dir).each do |child|
            if child == "#{dup_child_base}.mp3" || child.start_with?("#{dup_child_base}.")
              FileUtils.rm_f(File.join(dir, child))
            end
          end
        end
      end
    end

    next unless chosen_file

    current_fn = File.basename(chosen_file)
    src_base = File.basename(chosen_file, ".mp3")
    dst_base = File.basename(target_mp3, ".mp3")

    # Rename file if target differs
    if current_fn != target_fn
      puts "  [RENAME] #{current_fn} -> #{target_fn} (Pub: #{pub_time.strftime("%Y-%m-%d %H:%M")})"
      total_renamed += 1
      if execute
        # If target_mp3 exists and is not chosen_file, remove it
        if File.exist?(target_mp3) && target_mp3 != chosen_file
          FileUtils.rm_f(target_mp3)
        end
        FileUtils.mv(chosen_file, target_mp3)
        Dir.children(dir).each do |child|
          next if child == "#{src_base}.mp3"
          if child.start_with?("#{src_base}.")
            suffix = child[src_base.length..-1]
            FileUtils.mv(File.join(dir, child), File.join(dir, "#{dst_base}#{suffix}"))
          end
        end
        chosen_file = target_mp3
      end
    end

    # Update or create companion .mp3.json status
    status_file = execute ? "#{target_mp3}.json" : "#{chosen_file}.json"
    status_data = {}
    if File.exist?(status_file)
      begin
        status_data = JSON.parse(File.read(status_file))
      rescue StandardError
        status_data = {}
      end
    end

    old_pub = status_data["published_at"]
    new_pub = pub_time.iso8601

    if old_pub != new_pub
      puts "  [UPDATE DATE] #{File.basename(chosen_file)}: #{old_pub || "empty"} -> #{new_pub}"
      total_repaired += 1
      if execute
        status_data["published_at"] = new_pub
        status_data["publication_source"] = "feed"
        status_data["media_file"] = File.basename(target_mp3)
        status_data["original"] ||= {}
        status_data["original"]["filename"] = File.basename(target_mp3)
        File.write(status_file, JSON.pretty_generate(status_data))
      end
    end
  end
end

puts "\n=== Summary ==="
puts "Episodes with corrected publication date: #{total_repaired}"
puts "Episodes renamed with correct date prefix: #{total_renamed}"
puts "Duplicate files removed: #{total_duplicates_removed}"
unless execute
  puts "\nDry-run complete. Re-run with --execute (-e) to apply changes."
end
