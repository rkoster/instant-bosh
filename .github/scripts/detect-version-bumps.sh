#!/bin/bash
set -euo pipefail

# Detect version bumps in git diff and generate a commit message
# This script analyzes staged changes to find version updates
# Compatible with bash 3.2+ (no associative arrays)

# Check if there are staged changes
if ! git diff --cached --quiet; then
    # Get the diff of staged changes
    diff_output=$(git diff --cached)
    
    # Use parallel arrays instead of associative arrays for bash 3.2 compatibility
    components=()
    changes=()
    
    # Parse the diff to find version changes
    # Strategy: For each component, collect old version (from - lines) and new version (from + lines)
    current_component=""
    old_version=""
    
    while IFS= read -r line; do
        # Capture component name from any line containing "name:"
        if [[ "$line" =~ name:[[:space:]]*([a-zA-Z0-9_-]+) ]]; then
            # When we see a new component name, save any pending change from previous component
            if [[ -n "$current_component" ]] && [[ -n "$old_version" ]]; then
                # We have an old version but haven't found the new one yet, so reset
                old_version=""
            fi
            current_component="${BASH_REMATCH[1]}"
        fi
        
        # Collect old version from removed lines (-)
        if [[ "$line" =~ ^-.*version:[[:space:]]*([0-9]+\.[0-9]+\.[0-9]+) ]]; then
            old_version="${BASH_REMATCH[1]}"
        elif [[ "$line" =~ ^-.*url:.*[vV]?([0-9]+\.[0-9]+\.[0-9]+) ]]; then
            # Also capture version from URL if not already captured
            if [[ -z "$old_version" ]]; then
                old_version="${BASH_REMATCH[1]}"
            fi
        fi
        
        # Collect new version from added lines (+)
        if [[ "$line" =~ ^\+.*version:[[:space:]]*([0-9]+\.[0-9]+\.[0-9]+) ]]; then
            new_version="${BASH_REMATCH[1]}"
            if [[ -n "$current_component" ]] && [[ -n "$old_version" ]] && [[ "$old_version" != "$new_version" ]]; then
                components+=("$current_component")
                changes+=("$old_version → $new_version")
                old_version=""  # Reset for next component
            fi
        elif [[ "$line" =~ ^\+.*url:.*[vV]?([0-9]+\.[0-9]+\.[0-9]+) ]]; then
            new_version="${BASH_REMATCH[1]}"
            if [[ -n "$current_component" ]] && [[ -n "$old_version" ]] && [[ "$old_version" != "$new_version" ]]; then
                components+=("$current_component")
                changes+=("$old_version → $new_version")
                old_version=""  # Reset for next component
            fi
        fi
    done <<< "$diff_output"
    
    # Generate commit message
    if [ ${#components[@]} -gt 0 ]; then
        echo "chore: bump dependencies"
        echo ""
        for i in "${!components[@]}"; do
            echo "${components[$i]}: ${changes[$i]}"
        done
    else
        echo "chore: sync vendored dependencies"
    fi
else
    echo "No staged changes to commit"
    exit 1
fi
