import { describe, expect, it, vi } from "vitest";
import {
  boardPreferenceFor,
  clearRequestCaches,
  createRequestCache,
  pickOption,
  rememberedBoardStart,
} from "./board-discovery";

const remembered = {
  projectId: "Contoso",
  teamId: "team-atm",
  teamName: "Offers",
  boardId: "Microsoft.RequirementCategory",
  boardName: "Stories",
};

describe("rememberedBoardStart", () => {
  it("starts on the remembered team and board of the same project", () => {
    expect(rememberedBoardStart("Contoso", remembered)).toEqual({
      team: { id: "team-atm", name: "Offers" },
      board: { id: "Microsoft.RequirementCategory", name: "Stories" },
    });
  });

  it("does not start on a team remembered for another project", () => {
    expect(rememberedBoardStart("IT.EDP", remembered)).toBeNull();
  });

  it("needs the ids and names of both the team and the board", () => {
    expect(rememberedBoardStart("Contoso", undefined)).toBeNull();
    expect(rememberedBoardStart("Contoso", { ...remembered, boardName: "" })).toBeNull();
    expect(rememberedBoardStart("Contoso", { ...remembered, teamId: undefined })).toBeNull();
    const { projectId: _projectId, ...withoutProject } = remembered;
    expect(rememberedBoardStart("Contoso", withoutProject)).toBeNull();
  });
});

describe("pickOption", () => {
  const options = [
    { id: "a", name: "A" },
    { id: "b", name: "B" },
  ];

  it("keeps the current option when it is listed", () => {
    expect(pickOption(options, "b", "a")).toBe("b");
  });

  it("falls back to the remembered option, then to the first", () => {
    expect(pickOption(options, "gone", "b")).toBe("b");
    expect(pickOption(options, "", "gone")).toBe("a");
    expect(pickOption([], "a", "b")).toBe("");
  });
});

describe("createRequestCache", () => {
  it("shares one request per workspace and project", async () => {
    const fetchTeams = vi.fn(async (_workspaceId: string, projectId: string) => [
      { id: `${projectId}-1`, name: "One" },
    ]);
    const cache = createRequestCache(fetchTeams);
    const [first, second] = await Promise.all([cache.get("ws", "p1"), cache.get("ws", "p1")]);
    await cache.get("ws", "p1");
    await cache.get("ws", "p2");
    expect(first).toBe(second);
    expect(fetchTeams).toHaveBeenCalledTimes(2);
  });

  it("forgets a failed request so the next one retries", async () => {
    const fetchTeams = vi
      .fn<(workspaceId: string, projectId: string) => Promise<{ id: string; name: string }[]>>()
      .mockRejectedValueOnce(new Error("offline"))
      .mockResolvedValueOnce([{ id: "t", name: "T" }]);
    const cache = createRequestCache(fetchTeams);
    await expect(cache.get("ws", "p1")).rejects.toThrow("offline");
    await expect(cache.get("ws", "p1")).resolves.toEqual([{ id: "t", name: "T" }]);
  });

  it("reads again once a cached value is older than its lifetime", async () => {
    let now = 1_000;
    const fetchIdentity = vi
      .fn<(workspaceId: string) => Promise<string>>()
      .mockResolvedValueOnce("first account")
      .mockResolvedValueOnce("second account");
    const cache = createRequestCache(fetchIdentity, { ttlMs: 60_000, now: () => now });
    await expect(cache.get("ws")).resolves.toBe("first account");
    now += 59_999;
    await expect(cache.get("ws")).resolves.toBe("first account");
    now += 1;
    await expect(cache.get("ws")).resolves.toBe("second account");
    expect(fetchIdentity).toHaveBeenCalledTimes(2);
  });

  it("forgets every cached value when the caches are cleared", async () => {
    const fetchIdentity = vi
      .fn<(workspaceId: string) => Promise<string>>()
      .mockResolvedValueOnce("first account")
      .mockResolvedValueOnce("second account");
    const fetchTeams = vi.fn(async (workspaceId: string) => [workspaceId]);
    const identities = createRequestCache(fetchIdentity);
    const teams = createRequestCache(fetchTeams);
    await identities.get("ws");
    await teams.get("ws");
    clearRequestCaches();
    await expect(identities.get("ws")).resolves.toBe("second account");
    await teams.get("ws");
    expect(fetchTeams).toHaveBeenCalledTimes(2);
  });

  it("keeps a request started after a clear when the cleared one fails", async () => {
    let rejectFirst: (cause: Error) => void = () => undefined;
    const fetchIdentity = vi
      .fn<(workspaceId: string) => Promise<string>>()
      .mockImplementationOnce(() => new Promise<string>((_, reject) => (rejectFirst = reject)))
      .mockResolvedValueOnce("second account");
    const cache = createRequestCache(fetchIdentity);
    const first = cache.get("ws");
    cache.clear();
    await expect(cache.get("ws")).resolves.toBe("second account");
    rejectFirst(new Error("offline"));
    await expect(first).rejects.toThrow("offline");
    await expect(cache.get("ws")).resolves.toBe("second account");
    expect(fetchIdentity).toHaveBeenCalledTimes(2);
  });
});

describe("boardPreferenceFor", () => {
  it("remembers the project and the labels of the open team and board", () => {
    expect(
      boardPreferenceFor({
        projectId: "Contoso",
        teams: [{ id: "team-atm", name: "Offers" }],
        teamId: "team-atm",
        boards: [{ id: "Microsoft.RequirementCategory", name: "Stories" }],
        boardId: "Microsoft.RequirementCategory",
        focusedColumnId: "col-1",
      }),
    ).toEqual({ ...remembered, focusedColumnId: "col-1" });
  });

  it("round-trips through rememberedBoardStart", () => {
    const preference = boardPreferenceFor({
      projectId: "Contoso",
      teams: [{ id: "t", name: "Team" }],
      teamId: "t",
      boards: [{ id: "b", name: "Board" }],
      boardId: "b",
      focusedColumnId: "c",
    });
    expect(rememberedBoardStart("Contoso", preference)).toEqual({
      team: { id: "t", name: "Team" },
      board: { id: "b", name: "Board" },
    });
  });
});
