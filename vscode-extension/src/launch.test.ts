import {
  isLaunchable,
  resolveCwd,
  planStartAll,
  composeStartAllToast,
  composeTaskLaunchCommand,
  composeCloseTerminalsToast,
  selectVictimNames,
  resolveLoginShell,
  composeShellArgs,
  adhocIndexFromName,
  nextAdhocIndex,
  adhocDisplayName,
  adhocSessionArg,
  composeAdhocCommand,
  LaunchTask,
  StartAllPlan,
} from "./launch";
import { isAnyTaskTerminal } from "./terminals";
import { t, eq, deepEq, ok, notOk } from "./test-harness";

t.setModule("launch");

// Convenience builder for fixture tasks.
const task = (over: Partial<LaunchTask> = {}): LaunchTask => ({
  id: "TASK-00001",
  status: "backlog",
  ...over,
});

// fs-existence stubs: never-exists, always-exists, allowlist.
const noWorktree = (_: string): boolean => false;
const yesWorktree = (_: string): boolean => true;
const allow =
  (paths: string[]) =>
  (p: string): boolean =>
    paths.includes(p);

// ---- isLaunchable --------------------------------------------------------
t.testSync("isLaunchable: existing worktree → true", () => {
  ok(isLaunchable(task({ worktree_path: "/wt/T1" }), allow(["/wt/T1"])));
});
t.testSync(
  "isLaunchable: worktree-path string but not on disk + repo → true",
  () => {
    // Falls through to the repo branch.
    ok(
      isLaunchable(
        task({ worktree_path: "/wt/missing", repo: "https://github.com/a/b" }),
        noWorktree,
      ),
    );
  },
);
t.testSync("isLaunchable: worktree-path missing + no repo → false", () => {
  notOk(isLaunchable(task({ worktree_path: "/wt/missing" }), noWorktree));
});
t.testSync("isLaunchable: no worktree_path + repo present → true", () => {
  ok(isLaunchable(task({ repo: "https://github.com/a/b" }), noWorktree));
});
t.testSync("isLaunchable: no worktree_path + no repo → false", () => {
  notOk(isLaunchable(task({}), noWorktree));
});
t.testSync(
  "isLaunchable: empty-string worktree_path + repo → true (treated as absent)",
  () => {
    ok(isLaunchable(task({ worktree_path: "", repo: "x" }), yesWorktree));
  },
);
t.testSync("isLaunchable: empty-string repo + worktree on disk → true", () => {
  ok(
    isLaunchable(
      task({ worktree_path: "/wt/T1", repo: "" }),
      allow(["/wt/T1"]),
    ),
  );
});
t.testSync("isLaunchable: empty-string repo + no worktree → false", () => {
  notOk(isLaunchable(task({ repo: "" }), noWorktree));
});
t.testSync(
  "isLaunchable: nested worktree path is honored (TASK-00049 layout)",
  () => {
    // Regression guard: the extension must accept whatever worktree_path the
    // task carries — including nested paths like work/sub/TASK-00049 — and not
    // assume a flat work/TASK-id layout. This proves the predicate just calls
    // the injected existence-check on the literal path it was given.
    const nested = "/Users/v/Code/workspace/work/group/TASK-00049";
    ok(
      isLaunchable(
        task({ id: "TASK-00049", worktree_path: nested }),
        allow([nested]),
      ),
    );
  },
);

// ---- resolveCwd ----------------------------------------------------------
t.testSync("resolveCwd: existing worktree → returned", () => {
  eq(
    resolveCwd(task({ worktree_path: "/wt/T1" }), allow(["/wt/T1"]), "/home"),
    "/wt/T1",
  );
});
t.testSync("resolveCwd: missing worktree → adbHome fallback", () => {
  eq(
    resolveCwd(task({ worktree_path: "/wt/missing" }), noWorktree, "/home"),
    "/home",
  );
});
t.testSync("resolveCwd: no worktree_path → adbHome", () => {
  eq(resolveCwd(task({}), noWorktree, "/home"), "/home");
});
t.testSync("resolveCwd: missing worktree + undefined home → undefined", () => {
  eq(
    resolveCwd(task({ worktree_path: "/wt/missing" }), noWorktree, undefined),
    undefined,
  );
});
t.testSync("resolveCwd: worktree exists wins over home", () => {
  eq(
    resolveCwd(task({ worktree_path: "/wt/T1" }), yesWorktree, "/home"),
    "/wt/T1",
  );
});

// ---- planStartAll --------------------------------------------------------
const noneOpen = (_: LaunchTask): boolean => false;
const allOpen = (_: LaunchTask): boolean => true;

t.testSync("planStartAll: empty input → all zeros", () => {
  const plan = planStartAll([], noneOpen, noWorktree, 5);
  deepEq(plan, {
    toStart: [],
    launchable: 0,
    alreadyOpen: 0,
    deferred: 0,
    skipped: 0,
  });
});
t.testSync(
  "planStartAll: opens active tasks, excludes only terminal statuses",
  () => {
    // Changed behavior (TASK-00049 follow-up): backlog AND in_progress/review/
    // blocked all open; only done/archived are excluded.
    const tasks = [
      task({ id: "T1", status: "in_progress", repo: "x" }),
      task({ id: "T2", status: "done", repo: "x" }),
      task({ id: "T3", status: "backlog", repo: "x" }),
    ];
    const plan = planStartAll(tasks, noneOpen, noWorktree, 5);
    eq(plan.launchable, 2);
    eq(plan.toStart.length, 2);
    deepEq(
      plan.toStart.map((t) => t.id),
      ["T1", "T3"],
    );
  },
);
t.testSync(
  "planStartAll: counts skipped (backlog without worktree/repo)",
  () => {
    const tasks = [
      task({ id: "T1" }), // no worktree, no repo → skipped
      task({ id: "T2", repo: "x" }), // launchable via repo
      task({ id: "T3" }), // skipped
    ];
    const plan = planStartAll(tasks, noneOpen, noWorktree, 5);
    eq(plan.skipped, 2);
    eq(plan.launchable, 1);
    eq(plan.toStart.length, 1);
    eq(plan.toStart[0].id, "T2");
  },
);
t.testSync(
  "planStartAll: alreadyOpen excluded from toStart but counted",
  () => {
    const tasks = [
      task({ id: "T1", repo: "x" }),
      task({ id: "T2", repo: "x" }),
      task({ id: "T3", repo: "x" }),
    ];
    // T1 is already open
    const isOpen = (t: LaunchTask): boolean => t.id === "T1";
    const plan = planStartAll(tasks, isOpen, noWorktree, 5);
    eq(plan.launchable, 3);
    eq(plan.alreadyOpen, 1);
    eq(plan.toStart.length, 2);
    notOk(plan.toStart.some((t) => t.id === "T1"));
  },
);
t.testSync("planStartAll: cap boundary — exactly CAP", () => {
  const tasks = [1, 2, 3, 4, 5].map((i) => task({ id: `T${i}`, repo: "x" }));
  const plan = planStartAll(tasks, noneOpen, noWorktree, 5);
  eq(plan.launchable, 5);
  eq(plan.toStart.length, 5);
  eq(plan.deferred, 0);
});
t.testSync("planStartAll: cap boundary — CAP + 1", () => {
  const tasks = [1, 2, 3, 4, 5, 6].map((i) => task({ id: `T${i}`, repo: "x" }));
  const plan = planStartAll(tasks, noneOpen, noWorktree, 5);
  eq(plan.launchable, 6);
  eq(plan.toStart.length, 5);
  eq(plan.deferred, 1);
  // Order preserved.
  deepEq(
    plan.toStart.map((t) => t.id),
    ["T1", "T2", "T3", "T4", "T5"],
  );
});
t.testSync("planStartAll: cap 0 → unlimited (start all)", () => {
  const tasks = [task({ id: "T1", repo: "x" }), task({ id: "T2", repo: "x" })];
  const plan = planStartAll(tasks, noneOpen, noWorktree, 0);
  eq(plan.launchable, 2);
  eq(plan.toStart.length, 2);
  eq(plan.deferred, 0);
});
t.testSync("planStartAll: negative cap → unlimited (start all)", () => {
  const tasks = [task({ repo: "x" })];
  const plan = planStartAll(tasks, noneOpen, noWorktree, -3);
  eq(plan.toStart.length, 1);
  eq(plan.deferred, 0);
});
t.testSync("planStartAll: all already open → toStart empty, deferred 0", () => {
  const tasks = [task({ id: "T1", repo: "x" }), task({ id: "T2", repo: "x" })];
  const plan = planStartAll(tasks, allOpen, noWorktree, 5);
  eq(plan.launchable, 2);
  eq(plan.alreadyOpen, 2);
  eq(plan.toStart.length, 0);
  eq(plan.deferred, 0);
});
t.testSync(
  "planStartAll: mix of worktree-on-disk + repo-only + skipped",
  () => {
    const wtPath = "/wt/T1";
    const tasks = [
      task({ id: "T1", worktree_path: wtPath }), // launchable via worktree
      task({ id: "T2", repo: "x" }), // launchable via repo
      task({ id: "T3" }), // skipped (no worktree/repo/ticket_path)
      task({ id: "T4", status: "in_progress", repo: "x" }), // now also launchable
    ];
    const plan = planStartAll(tasks, noneOpen, allow([wtPath]), 5);
    eq(plan.launchable, 3);
    eq(plan.skipped, 1);
    eq(plan.toStart.length, 3);
    deepEq(
      plan.toStart.map((t) => t.id),
      ["T1", "T2", "T4"],
    );
  },
);
t.testSync("planStartAll: alreadyOpen does NOT count toward cap", () => {
  // 4 already-open + 5 not-open + cap 5 → all 5 not-open in toStart, 0 deferred.
  const tasks: LaunchTask[] = [];
  for (let i = 1; i <= 4; i++) {
    tasks.push(task({ id: `OPEN${i}`, repo: "x" }));
  }
  for (let i = 1; i <= 5; i++) {
    tasks.push(task({ id: `NEW${i}`, repo: "x" }));
  }
  const isOpen = (t: LaunchTask): boolean => t.id.startsWith("OPEN");
  const plan = planStartAll(tasks, isOpen, noWorktree, 5);
  eq(plan.launchable, 9);
  eq(plan.alreadyOpen, 4);
  eq(plan.toStart.length, 5);
  eq(plan.deferred, 0);
  notOk(plan.toStart.some((t) => t.id.startsWith("OPEN")));
});

// ---- NEW: "open ALL tickets, worktree-dir else ticket-dir" behavior -------
// (TASK-00049 follow-up: Start All should open every non-terminal ticket, and
// a ticket with no worktree but a ticket_path on disk is launchable from that
// ticket dir.)
t.testSync("isLaunchable: no worktree + no repo + ticket_path → true", () => {
  // A repo-less / hand-named ticket with only a ticket directory is still
  // launchable: we open claude in the ticket dir.
  ok(
    isLaunchable(
      task({ ticket_path: "/tickets/_local/personal-1" }),
      noWorktree,
    ),
  );
});
t.testSync(
  "isLaunchable: no worktree + no repo + no ticket_path → false",
  () => {
    notOk(isLaunchable(task({}), noWorktree));
  },
);
t.testSync("resolveCwd: no worktree → ticket_path before adbHome", () => {
  eq(
    resolveCwd(
      task({ worktree_path: "/wt/missing", ticket_path: "/tickets/T1" }),
      noWorktree,
      "/home",
    ),
    "/tickets/T1",
  );
});
t.testSync("resolveCwd: no worktree + no ticket_path → adbHome", () => {
  eq(resolveCwd(task({}), noWorktree, "/home"), "/home");
});
t.testSync("resolveCwd: existing worktree wins over ticket_path", () => {
  eq(
    resolveCwd(
      task({ worktree_path: "/wt/T1", ticket_path: "/tickets/T1" }),
      yesWorktree,
      "/home",
    ),
    "/wt/T1",
  );
});
t.testSync(
  "planStartAll: opens non-backlog tasks too (in_progress, review)",
  () => {
    // The whole point of the change: a click should open every ticket that has
    // somewhere to launch from, regardless of backlog vs in_progress.
    const tasks = [
      task({ id: "T1", status: "in_progress", repo: "x" }),
      task({ id: "T2", status: "review", ticket_path: "/tickets/T2" }),
      task({ id: "T3", status: "backlog", repo: "x" }),
    ];
    const plan = planStartAll(tasks, noneOpen, noWorktree, 0);
    eq(plan.launchable, 3);
    eq(plan.toStart.length, 3);
  },
);
t.testSync("planStartAll: excludes terminal statuses (done, archived)", () => {
  const tasks = [
    task({ id: "T1", status: "done", repo: "x" }),
    task({ id: "T2", status: "archived", repo: "x" }),
    task({ id: "T3", status: "in_progress", repo: "x" }),
  ];
  const plan = planStartAll(tasks, noneOpen, noWorktree, 0);
  eq(plan.launchable, 1);
  eq(plan.toStart.length, 1);
  eq(plan.toStart[0].id, "T3");
});
t.testSync("planStartAll: ticket-only task is launchable, not skipped", () => {
  const tasks = [
    task({ id: "T1", status: "in_progress", ticket_path: "/tickets/T1" }),
    task({ id: "T2", status: "in_progress" }), // truly nothing → skipped
  ];
  const plan = planStartAll(tasks, noneOpen, noWorktree, 0);
  eq(plan.launchable, 1);
  eq(plan.skipped, 1);
  eq(plan.toStart[0].id, "T1");
});

// ---- composeTaskLaunchCommand --------------------------------------------
t.testSync("launchCmd: worktree-bearing task → adb resume --here", () => {
  eq(
    composeTaskLaunchCommand(
      task({ id: "TASK-00001", worktree_path: "/wt/T1" }),
      "/usr/local/bin/adb",
    ),
    "/usr/local/bin/adb task resume TASK-00001 --here",
  );
});
t.testSync(
  "launchCmd: repo-only task → adb resume --here (resume clones)",
  () => {
    eq(
      composeTaskLaunchCommand(task({ id: "TASK-00002", repo: "x" }), "adb"),
      "adb task resume TASK-00002 --here",
    );
  },
);
t.testSync(
  "launchCmd: ticket-only task → adb resume --here (tmux-hosts via ticket dir)",
  () => {
    // CHANGED (TASK-00023): ticket-only tasks no longer launch bare claude. The
    // adb CLI falls back to the ticket dir for repo-less tasks and hosts claude in
    // a survivable tmux session, so every ticket gets its own cc-TASK-NNNNN session
    // and a uniform --here launch path (no pty-host-child that dies on reload).
    eq(
      composeTaskLaunchCommand(
        task({ id: "TASK-00009", ticket_path: "/tickets/_local/00009" }),
        "adb",
      ),
      "adb task resume TASK-00009 --here",
    );
  },
);
t.testSync(
  "launchCmd: task with NOTHING → still adb resume --here (uniform path)",
  () => {
    // Even a task with no worktree/repo/ticket_path composes the same command;
    // launchability is gated separately by isLaunchable/planStartAll, not here.
    eq(
      composeTaskLaunchCommand(
        task({ id: "TASK-00099" }),
        "/usr/local/bin/adb",
      ),
      "/usr/local/bin/adb task resume TASK-00099 --here",
    );
  },
);

// ---- resolveLoginShell ---------------------------------------------------
t.testSync("resolveLoginShell: uses $SHELL when set (posix)", () => {
  eq(resolveLoginShell({ SHELL: "/usr/bin/zsh" }, "linux"), "/usr/bin/zsh");
});
t.testSync(
  "resolveLoginShell: falls back to /bin/bash when SHELL unset (posix)",
  () => {
    eq(resolveLoginShell({}, "darwin"), "/bin/bash");
  },
);
t.testSync(
  "resolveLoginShell: falls back to /bin/bash when SHELL empty (posix)",
  () => {
    eq(resolveLoginShell({ SHELL: "" }, "linux"), "/bin/bash");
  },
);
t.testSync(
  "resolveLoginShell: Windows uses PowerShell, ignoring $SHELL (#227)",
  () => {
    // /bin/bash is not a valid Windows shellPath; a stray $SHELL must not win.
    eq(resolveLoginShell({}, "win32"), "powershell.exe");
    eq(resolveLoginShell({ SHELL: "/bin/bash" }, "win32"), "powershell.exe");
  },
);

// ---- composeShellArgs ----------------------------------------------------
t.testSync(
  "composeShellArgs: login shell + -c command + exec-shell tail (posix)",
  () => {
    deepEq(
      composeShellArgs(
        "adb task resume TASK-00009 --here",
        "/usr/bin/zsh",
        "linux",
      ),
      ["-l", "-c", "adb task resume TASK-00009 --here; exec /usr/bin/zsh -l"],
    );
  },
);
t.testSync(
  "composeShellArgs: bash fallback shell in the exec tail (posix)",
  () => {
    deepEq(
      composeShellArgs(
        "adb task resume TASK-00001 --here",
        "/bin/bash",
        "darwin",
      ),
      ["-l", "-c", "adb task resume TASK-00001 --here; exec /bin/bash -l"],
    );
  },
);
t.testSync(
  "composeShellArgs: Windows hosts the command in PowerShell -NoExit (#227)",
  () => {
    deepEq(
      composeShellArgs(
        "adb task resume TASK-00009 --here",
        "powershell.exe",
        "win32",
      ),
      ["-NoExit", "-Command", "adb task resume TASK-00009 --here"],
    );
  },
);
t.testSync(
  "composeShellArgs: command is preserved verbatim (no escaping mangling)",
  () => {
    // The command comes from composeTaskLaunchCommand (adb + task id) — known-safe
    // tokens. Assert we pass it through unchanged inside the -c string.
    const argv = composeShellArgs(
      "adb task resume TASK-00042 --here",
      "/usr/bin/zsh",
      "linux",
    );
    eq(argv[0], "-l");
    eq(argv[1], "-c");
    ok(argv[2].startsWith("adb task resume TASK-00042 --here; exec "));
  },
);

// ---- adhocIndexFromName --------------------------------------------------
t.testSync("adhocIndexFromName: bare '🌙 claude' → 1", () => {
  eq(adhocIndexFromName("🌙 claude"), 1);
});
t.testSync("adhocIndexFromName: '🌙 claude 2' → 2", () => {
  eq(adhocIndexFromName("🌙 claude 2"), 2);
});
t.testSync("adhocIndexFromName: '🌙 claude 17' → 17 (multi-digit)", () => {
  eq(adhocIndexFromName("🌙 claude 17"), 17);
});
t.testSync("adhocIndexFromName: task terminal → undefined", () => {
  eq(adhocIndexFromName("TASK-00005 feat P2"), undefined);
});
t.testSync("adhocIndexFromName: plain shell → undefined", () => {
  eq(adhocIndexFromName("zsh"), undefined);
});
t.testSync("adhocIndexFromName: similar-but-wrong prefix → undefined", () => {
  eq(adhocIndexFromName("🌙 claudex"), undefined);
  eq(adhocIndexFromName("claude 2"), undefined);
  eq(adhocIndexFromName("🌙 claude two"), undefined);
});

// ---- nextAdhocIndex ------------------------------------------------------
t.testSync("nextAdhocIndex: no terminals → 1", () => {
  eq(nextAdhocIndex([]), 1);
});
t.testSync("nextAdhocIndex: only non-adhoc terminals → 1", () => {
  eq(nextAdhocIndex(["zsh", "TASK-00005 feat P2", "ADB Dashboard"]), 1);
});
t.testSync("nextAdhocIndex: 1 open → 2", () => {
  eq(nextAdhocIndex(["🌙 claude"]), 2);
});
t.testSync("nextAdhocIndex: 1 and 2 open → 3", () => {
  eq(nextAdhocIndex(["🌙 claude", "🌙 claude 2"]), 3);
});
t.testSync("nextAdhocIndex: fills the lowest GAP (2 closed) → 2", () => {
  // 1 and 3 open, 2 was closed → reuse 2, not 4.
  eq(nextAdhocIndex(["🌙 claude", "🌙 claude 3"]), 2);
});
t.testSync(
  "nextAdhocIndex: ignores task/other terminals when numbering",
  () => {
    eq(
      nextAdhocIndex([
        "🌙 claude",
        "TASK-00009 feat P1",
        "🌙 claude 2",
        "bash",
      ]),
      3,
    );
  },
);
t.testSync("nextAdhocIndex: order-independent", () => {
  eq(nextAdhocIndex(["🌙 claude 3", "🌙 claude", "🌙 claude 2"]), 4);
});

// ---- adhocDisplayName / adhocSessionArg ----------------------------------
t.testSync("adhocDisplayName: 1 → bare label (no number)", () => {
  eq(adhocDisplayName(1), "🌙 claude");
});
t.testSync("adhocDisplayName: n>1 → numbered label", () => {
  eq(adhocDisplayName(2), "🌙 claude 2");
  eq(adhocDisplayName(10), "🌙 claude 10");
});
t.testSync(
  "adhocSessionArg: index → adhoc-<n> (cc-survive prepends cc-)",
  () => {
    eq(adhocSessionArg(1), "adhoc-1");
    eq(adhocSessionArg(7), "adhoc-7");
  },
);
t.testSync(
  "adhoc round-trip: displayName index is recoverable by adhocIndexFromName",
  () => {
    for (const i of [1, 2, 5, 42]) {
      eq(adhocIndexFromName(adhocDisplayName(i)), i);
    }
  },
);

// ---- composeAdhocCommand -------------------------------------------------
t.testSync("composeAdhocCommand: posix → cc-survive <sessionArg>", () => {
  eq(composeAdhocCommand("adhoc-1", "linux"), "cc-survive adhoc-1");
  eq(composeAdhocCommand("adhoc-3", "darwin"), "cc-survive adhoc-3");
});
t.testSync(
  "composeAdhocCommand: Windows → bare claude (cc-survive/tmux is Unix-only)",
  () => {
    // cc-survive is a Unix/tmux wrapper that can't run under PowerShell, and MSYS
    // tmux denies claude a console pty on Windows — so the ad-hoc press degrades to
    // a direct claude launch, the same non-survivable fallback the CLI applies for
    // task terminals on win32. The sessionArg is irrelevant (no tmux session to
    // name), so any index maps to the same command.
    eq(
      composeAdhocCommand("adhoc-1", "win32"),
      "claude --dangerously-skip-permissions",
    );
    eq(
      composeAdhocCommand("adhoc-9", "win32"),
      "claude --dangerously-skip-permissions",
    );
  },
);
t.testSync(
  "composeAdhocCommand: posix wraps cleanly into composeShellArgs",
  () => {
    // The integration the runtime uses: shellArgs runs cc-survive then drops to a
    // login shell, so the tab survives detach AND the command re-runs on revival.
    const cmd = composeAdhocCommand(adhocSessionArg(2), "linux");
    deepEq(composeShellArgs(cmd, "/usr/bin/zsh", "linux"), [
      "-l",
      "-c",
      "cc-survive adhoc-2; exec /usr/bin/zsh -l",
    ]);
  },
);
t.testSync(
  "composeAdhocCommand: Windows wraps into PowerShell -NoExit (working tab)",
  () => {
    // End-to-end for a Windows ad-hoc press: PowerShell hosts a bare claude and
    // stays open (-NoExit), so the tab is usable and VS Code re-runs it on reload
    // (a fresh session — Windows has no tmux survivability). This is the analogue
    // of the POSIX `cc-survive …; exec <shell> -l` tail.
    const cmd = composeAdhocCommand(adhocSessionArg(1), "win32");
    deepEq(composeShellArgs(cmd, "powershell.exe", "win32"), [
      "-NoExit",
      "-Command",
      "claude --dangerously-skip-permissions",
    ]);
  },
);

// ---- composeStartAllToast ------------------------------------------------
const plan = (over: Partial<StartAllPlan> = {}): StartAllPlan => ({
  toStart: [],
  launchable: 0,
  alreadyOpen: 0,
  deferred: 0,
  skipped: 0,
  ...over,
});

t.testSync("toast: launchable=0, no skipped → bare 'no launchable'", () => {
  eq(composeStartAllToast(plan(), 0), "ADB: no launchable tasks");
});
t.testSync("toast: launchable=0, skipped>0 → includes skipped detail", () => {
  eq(
    composeStartAllToast(plan({ skipped: 3 }), 0),
    "ADB: no launchable tasks (3 task(s) have no worktree, repo, or ticket dir to launch)",
  );
});
t.testSync("toast: started>0, no notes → plain success", () => {
  eq(
    composeStartAllToast(plan({ launchable: 2 }), 2),
    "ADB: started 2 task(s) in terminals",
  );
});
t.testSync("toast: started>0 + deferred → success with note", () => {
  eq(
    composeStartAllToast(plan({ launchable: 7, deferred: 2 }), 5),
    "ADB: started 5 task(s) in terminals — 2 more launchable (re-run to continue)",
  );
});
t.testSync("toast: started=0 + alreadyOpen → 'nothing new to start'", () => {
  eq(
    composeStartAllToast(plan({ launchable: 3, alreadyOpen: 3 }), 0),
    "ADB: nothing new to start — 3 already open",
  );
});
t.testSync("toast: started=0 + skipped + alreadyOpen", () => {
  // launchable>0 because alreadyOpen counts as launchable; skipped is separate.
  eq(
    composeStartAllToast(
      plan({ launchable: 1, alreadyOpen: 1, skipped: 2 }),
      0,
    ),
    "ADB: nothing new to start — 1 already open, 2 skipped (no worktree/repo)",
  );
});
t.testSync("toast: started>0 + all three notes — order preserved", () => {
  eq(
    composeStartAllToast(
      plan({ launchable: 9, deferred: 2, alreadyOpen: 1, skipped: 3 }),
      5,
    ),
    "ADB: started 5 task(s) in terminals — 2 more launchable (re-run to continue), 1 already open, 3 skipped (no worktree/repo)",
  );
});
t.testSync(
  "toast: started=0 with no notes → success branch (edge: shouldn't happen but covered)",
  () => {
    // started===0 but launchable>0 and no notes is contradictory in practice
    // (launchable=2 → at least 2 alreadyOpen|deferred|skipped). But we still
    // exercise the branch: when notes.length is 0, we always take the success
    // branch — even with started=0.
    eq(
      composeStartAllToast(plan({ launchable: 2 }), 0),
      "ADB: started 0 task(s) in terminals",
    );
  },
);

// ---- composeCloseTerminalsToast ------------------------------------------
t.testSync("closeTerminalsToast: zero → 'no task terminals open'", () => {
  eq(composeCloseTerminalsToast(0), "ADB: no task terminals open");
});
t.testSync("closeTerminalsToast: one", () => {
  eq(composeCloseTerminalsToast(1), "ADB: closed 1 task terminal(s)");
});
t.testSync("closeTerminalsToast: many", () => {
  eq(composeCloseTerminalsToast(7), "ADB: closed 7 task terminal(s)");
});

// ---- selectVictimNames ---------------------------------------------------
t.testSync("selectVictimNames: keeps task terminals only", () => {
  const names = [
    "TASK-00005 refactor P2",
    "toolbox-exec",
    "TASK-00031 bug P1",
    "ADB Dashboard",
    "bash",
  ];
  deepEq(selectVictimNames(names, isAnyTaskTerminal), [
    "TASK-00005 refactor P2",
    "TASK-00031 bug P1",
  ]);
});
t.testSync("selectVictimNames: empty input → empty output", () => {
  deepEq(selectVictimNames([], isAnyTaskTerminal), []);
});
t.testSync("selectVictimNames: never selects toolbox-exec or dashboard", () => {
  const names = ["toolbox-exec", "ADB Dashboard", "bash"];
  deepEq(selectVictimNames(names, isAnyTaskTerminal), []);
});
t.testSync("selectVictimNames: preserves order from input", () => {
  const names = ["TASK-00031 bug P1", "toolbox-exec", "TASK-00005 refactor P2"];
  deepEq(selectVictimNames(names, isAnyTaskTerminal), [
    "TASK-00031 bug P1",
    "TASK-00005 refactor P2",
  ]);
});
