#!/usr/bin/env bash
# Reports the state of every linked git worktree, so the decision to remove one
# is made from facts rather than from memory. Each column is one command:
#
#   MERGED       git rev-list --count main..<branch>, which is zero exactly
#                when main already holds everything on the branch. That is the
#                same question `git branch --merged` answers, and the count
#                also says HOW MUCH is missing when the answer is no.
#   LAST COMMIT  git log -1 --format=%cr
#   TREE         git status --porcelain
#
# It REMOVES NOTHING, on purpose. A worktree that is merged and clean is safe
# by construction, but "safe" is not "finished": a branch nobody has reviewed
# looks exactly like one that was abandoned, and no command tells them apart.
# Removing is left to the reader, who knows which is which.
set -euo pipefail

cd "$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

main_branch="main"
found=0
stale=0

worktree=""
branch=""
detached=0
prunable=""

# reset clears the fields between entries, so a field the next entry omits is
# not read as belonging to it.
reset() {
	worktree=""
	branch=""
	detached=0
	prunable=""
}

report() {
	local name age missing merged tree verdict

	name="$(basename "$worktree")"

	# A prunable worktree has no directory left to ask anything of, so it is
	# answered before anything tries.
	if [ -n "$prunable" ]; then
		stale=1
		printf '%-12s %-24s %-7s %-16s %-7s %s\n' \
			"$name" "${branch:-(gone)}" "-" "-" "-" "stale registration: $prunable"
		return 0
	fi

	age="$(git -C "$worktree" log -1 --format=%cr 2>/dev/null || echo unknown)"
	tree=clean
	if [ -n "$(git -C "$worktree" status --porcelain)" ]; then
		tree=dirty
	fi

	# Counted from HEAD rather than from the branch name, so a detached worktree
	# is judged on the same terms as any other instead of being set aside.
	missing="$(git -C "$worktree" rev-list --count "$main_branch..HEAD" 2>/dev/null || echo '?')"
	merged=no
	if [ "$missing" = 0 ]; then
		merged=yes
	fi

	# Uncommitted work outranks everything else: it exists nowhere but here, so
	# no state of the branch makes the directory safe to delete.
	if [ "$tree" = dirty ]; then
		verdict="uncommitted changes"
		if [ "$merged" = no ]; then
			verdict="$verdict, and $missing commit(s) not in $main_branch"
		fi
	elif [ "$merged" = no ]; then
		verdict="$missing commit(s) not in $main_branch"
	else
		# Deliberately not "no commits of its own": a branch that never
		# committed and one whose commits are all merged look identical here,
		# and claiming to tell them apart would be a guess.
		verdict="safe to remove — $main_branch holds everything on it"
	fi

	printf '%-12s %-24s %-7s %-16s %-7s %s\n' \
		"$name" "${branch:-(detached)}" "$merged" "$age" "$tree" "$verdict"
}

# git lists the main worktree first, whatever directory this runs from. It is
# the repository itself and is never a candidate for removal, so it is skipped.
skip_main=1

while IFS= read -r line; do
	case "$line" in
	worktree\ *) worktree="${line#worktree }" ;;
	branch\ *) branch="${line#branch refs/heads/}" ;;
	detached) detached=1 ;;
	prunable*) prunable="${line#prunable }" ;;
	"")
		if [ -n "$worktree" ]; then
			if [ "$skip_main" = 1 ]; then
				skip_main=0
			else
				if [ "$found" = 0 ]; then
					printf '%-12s %-24s %-7s %-16s %-7s %s\n' \
						DIRECTORY BRANCH MERGED "LAST COMMIT" TREE VERDICT
				fi
				found=$((found + 1))
				report
			fi
		fi
		reset
		;;
	esac
done < <(git worktree list --porcelain; echo)

if [ "$found" = 0 ]; then
	echo "No linked worktrees."
	exit 0
fi

echo
echo "This removes nothing. To remove one:"
echo "  git worktree remove worktrees/<name> && git branch -d <branch>"
if [ "$stale" = 1 ]; then
	echo "To drop a stale registration, whose directory is already gone:"
	echo "  git worktree prune"
fi
