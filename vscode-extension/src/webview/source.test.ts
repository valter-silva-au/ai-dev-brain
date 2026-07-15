// Tests for the pure line-splitter that FeedSource impls share. The seam
// itself is an interface (no runtime), but splitLines is the single spot
// where "did the chunk end mid-line?" is decided — and getting that wrong
// would corrupt every event past the first split. Pin it explicitly.
import { t, deepEq, eq } from "../test-harness";
import { splitLines } from "./source";

t.setModule("source");

t.testSync("splitLines: single complete line → one line, empty held", () => {
  const r = splitLines("{\"type\":\"task.created\"}\n", "");
  deepEq(r.lines, ["{\"type\":\"task.created\"}"]);
  eq(r.held, "");
});

t.testSync("splitLines: multiple complete lines → all emitted, empty held", () => {
  const r = splitLines("a\nb\nc\n", "");
  deepEq(r.lines, ["a", "b", "c"]);
  eq(r.held, "");
});

t.testSync("splitLines: incomplete tail → held for next chunk", () => {
  const r = splitLines("a\nb\nc", "");
  deepEq(r.lines, ["a", "b"]);
  eq(r.held, "c");
});

t.testSync("splitLines: prior held is prepended to the new chunk", () => {
  const r = splitLines("world\n", "hello ");
  deepEq(r.lines, ["hello world"]);
  eq(r.held, "");
});

t.testSync("splitLines: empty chunk with prior held → nothing emitted, held preserved", () => {
  const r = splitLines("", "abc");
  deepEq(r.lines, []);
  eq(r.held, "abc");
});

t.testSync("splitLines: CRLF handled the same as LF", () => {
  const r = splitLines("a\r\nb\r\n", "");
  deepEq(r.lines, ["a", "b"]);
});

t.testSync("splitLines: no newlines at all → whole chunk becomes held", () => {
  const r = splitLines("partial-line-no-newline", "");
  deepEq(r.lines, []);
  eq(r.held, "partial-line-no-newline");
});
