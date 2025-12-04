#!/bin/bash
set -euo pipefail

# Detect version bumps in git diff and generate a commit message
# This script analyzes staged changes to find version updates

# Check if there are staged changes
if ! git diff --cached --quiet; then
    # Get the diff of staged changes
    diff_output=$(git diff --cached)
    
    # Initialize arrays to store component updates
    declare -A version_changes
    
    # Parse the diff to find version changes
    # Look for patterns like: -  version: 1.4.22 followed by +  version: 1.4.23
    current_component=""
    old_version=""
    
    while IFS= read -r line; do
        # Detect component name from releases section
        if [[ "$line" =~ ^[-+]?[[:space:]]*-[[:space:]]*name:[[:space:]]*(.+) ]]; then
            current_component="${BASH_REMATCH[1]}"
        fi
        
        # Detect old version (removed line)
        if [[ "$line" =~ ^-[[:space:]]*version:[[:space:]]*([0-9]+\.[0-9]+\.[0-9]+) ]]; then
            old_version="${BASH_REMATCH[1]}"
        fi
        
        # Detect new version (added line)
        if [[ "$line" =~ ^+[[:space:]]*version:[[:space:]]*([0-9]+\.[0-9]+\.[0-9]+) ]]; then
            new_version="${BASH_REMATCH[1]}"
            if [[ -n "$current_component" ]] && [[ -n "$old_version" ]] && [[ "$old_version" != "$new_version" ]]; then
                version_changes["$current_component"]="$old_version → $new_version"
                old_version=""
            fi
        fi
        
        # Also check for version in URL patterns (for CPI releases)
        if [[ "$line" =~ ^-[[:space:]]*url:.*[vV]?([0-9]+\.[0-9]+\.[0-9]+) ]]; then
            old_version="${BASH_REMATCH[1]}"
        fi
        
        if [[ "$line" =~ ^+[[:space:]]*url:.*[vV]?([0-9]+\.[0-9]+\.[0-9]+) ]]; then
            new_version="${BASH_REMATCH[1]}"
            if [[ -n "$current_component" ]] && [[ -n "$old_version" ]] && [[ "$old_version" != "$new_version" ]]; then
                version_changes["$current_component"]="$old_version → $new_version"
                old_version=""
            fi
        fi
    done <<< "$diff_output"
    
    # Generate commit message
    if [ ${#version_changes[@]} -gt 0 ]; then
        echo "chore: bump dependencies"
        echo ""
        for component in "${!version_changes[@]}"; do
            echo "${component}: ${version_changes[$component]}"
        done
    else
        echo "chore: sync vendored dependencies"
    fi
else
    echo "No staged changes to commit"
    exit 1
fi
