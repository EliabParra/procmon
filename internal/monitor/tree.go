package monitor

// Connectors drawn in front of a command; each is three columns wide.
const (
	branchTee  = "├─ " // a child with more siblings after it
	branchLast = "└─ " // the last child
	branchBar  = "│  " // continues the line of an ancestor that has more siblings
	branchGap  = "   " // an ancestor that was the last child
)

// TreeRow is one line of the process tree view.
type TreeRow struct {
	Process Process
	Depth   int    // 0 for a root
	Prefix  string // box-drawing connectors to print before the command
}

// BuildTree flattens the parent -> child relation into display order, parents
// before their children. A process whose parent is not in procs (PID 1,
// kthreadd, an orphan) is a root. Roots and siblings keep the order of procs,
// so sorting the list first (by PID, CPU, memory) also sorts the tree.
// Every process appears exactly once even if the PPIDs form a cycle.
func BuildTree(procs []Process) []TreeRow {
	index := make(map[int]int, len(procs)) // pid -> position in procs
	for i, p := range procs {
		index[p.PID] = i
	}

	children := make(map[int][]int) // position of a parent -> positions of its children
	var roots []int
	for i, p := range procs {
		if parent, ok := index[p.PPID]; ok && parent != i {
			children[parent] = append(children[parent], i)
		} else {
			roots = append(roots, i)
		}
	}

	rows := make([]TreeRow, 0, len(procs))
	visited := make([]bool, len(procs))

	// walk emits procs[i] and then its subtree. indent is what the ancestors
	// contribute to every line below them; connector belongs to this line only.
	var walk func(i, depth int, indent, connector string)
	walk = func(i, depth int, indent, connector string) {
		if visited[i] {
			return
		}
		visited[i] = true
		rows = append(rows, TreeRow{Process: procs[i], Depth: depth, Prefix: indent + connector})

		below := indent
		switch connector {
		case branchTee:
			below += branchBar
		case branchLast:
			below += branchGap
		}
		kids := children[i]
		for k, child := range kids {
			c := branchTee
			if k == len(kids)-1 {
				c = branchLast
			}
			walk(child, depth+1, below, c)
		}
	}

	for _, r := range roots {
		walk(r, 0, "", "")
	}
	// What is left is a cycle (or hangs from one): no process in it is
	// anybody's root, so the first one in list order becomes one.
	for i := range procs {
		walk(i, 0, "", "")
	}
	return rows
}

// WithAncestors returns the processes of subset together with every ancestor
// of them that is listed in all, in the order of all. A filtered tree uses it
// to keep the hierarchy readable: a match is shown under the processes that
// started it. A parent cycle ends the walk instead of looping.
func WithAncestors(all, subset []Process) []Process {
	parent := make(map[int]int, len(all)) // pid -> ppid
	for _, p := range all {
		parent[p.PID] = p.PPID
	}
	keep := make(map[int]bool, len(subset))
	for _, p := range subset {
		// Stop at a process already kept: its own ancestors were kept when it
		// was, and that is also what ends a cycle.
		for pid := p.PID; !keep[pid]; {
			keep[pid] = true
			ppid, listed := parent[pid]
			if !listed {
				break
			}
			pid = ppid
		}
	}

	out := make([]Process, 0, len(keep))
	for _, p := range all {
		if keep[p.PID] {
			out = append(out, p)
		}
	}
	return out
}
