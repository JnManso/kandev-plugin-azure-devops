import { describe, expect, it, vi } from "vitest";
import { createSearchSequence, settleScopedSearch } from "./search-sequence";

function deferred<T>() {
  let resolve: (value: T) => void = () => undefined;
  let reject: (cause: unknown) => void = () => undefined;
  const promise = new Promise<T>((onResolve, onReject) => {
    resolve = onResolve;
    reject = onReject;
  });
  return { promise, resolve, reject };
}

describe("scoped searches", () => {
  it("drops an older team-scoped search that resolves after a newer one", async () => {
    const sequence = createSearchSequence();
    const run = vi.fn();
    const older = deferred<string>();
    const olderSettled = settleScopedSearch(older.promise, sequence.begin(), run, vi.fn());
    const newerSettled = settleScopedSearch(Promise.resolve("newer"), sequence.begin(), run, vi.fn());
    await newerSettled;
    older.resolve("older");
    await olderSettled;
    expect(run.mock.calls).toEqual([["newer"]]);
  });

  it("drops the result and the error of a search retired by unmount", async () => {
    const sequence = createSearchSequence();
    const run = vi.fn();
    const fail = vi.fn();
    const resolved = deferred<string>();
    const rejected = deferred<string>();
    const first = settleScopedSearch(resolved.promise, sequence.begin(), run, fail);
    sequence.retire();
    resolved.resolve("late");
    await first;
    const second = settleScopedSearch(rejected.promise, sequence.begin(), run, fail);
    sequence.retire();
    rejected.reject(new Error("offline"));
    await second;
    expect(run).not.toHaveBeenCalled();
    expect(fail).not.toHaveBeenCalled();
  });

  it("reports the error of the current search", async () => {
    const sequence = createSearchSequence();
    const fail = vi.fn();
    await settleScopedSearch(Promise.reject(new Error("offline")), sequence.begin(), vi.fn(), fail);
    expect(fail).toHaveBeenCalledWith(new Error("offline"));
  });
});
