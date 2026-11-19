import { describe, expect, it } from "vitest";
import {
  AZURE_PULL_REQUEST_PRESETS,
  AZURE_WORK_ITEM_PRESETS,
  advancedFilterCount,
  matchingPresetId,
  presetsForKind,
} from "./presets";

describe("Azure DevOps presets", () => {
  it("uses Azure WIQL identity macros for personal work-item presets", () => {
    expect(
      AZURE_WORK_ITEM_PRESETS.find((preset) => preset.value === "assigned")?.filters.wiql,
    ).toContain("[System.AssignedTo] = @Me");
    expect(
      AZURE_WORK_ITEM_PRESETS.find((preset) => preset.value === "created")?.filters.wiql,
    ).toContain("[System.CreatedBy] = @Me");
  });

  it("uses the backend @me identity sentinel for personal pull-request presets", () => {
    expect(
      AZURE_PULL_REQUEST_PRESETS.find((preset) => preset.value === "review-requested")?.filters,
    ).toMatchObject({ status: "active", reviewer: "@me" });
    expect(
      AZURE_PULL_REQUEST_PRESETS.find((preset) => preset.value === "created")?.filters,
    ).toMatchObject({ status: "active", creator: "@me" });
  });

  it("maps workspace-resolved Azure queries into browse shortcuts", () => {
    const presets = presetsForKind("pull_request", {
      workItems: [],
      pullRequests: [
        {
          id: "team-review",
          label: "Team review",
          group: "created",
          filters: { status: "active", reviewer: "team-id", creator: "@me" },
        },
      ],
    });

    expect(presets).toHaveLength(1);
    expect(presets[0]).toMatchObject({
      value: "team-review",
      label: "Team review",
      group: "created",
      filters: { status: "active", reviewer: "team-id", creator: "@me" },
    });
  });

  it("translates unchanged built-in labels but preserves customized labels", () => {
    const translate = (key: string) => `translated:${key}`;
    expect(presetsForKind("work_item", undefined, translate)[0].label).toBe(
      "translated:defaultQueryRecentlyUpdated",
    );

    const customized = presetsForKind(
      "work_item",
      {
        workItems: [
          {
            id: "recent",
            label: "My recent work",
            group: "inbox",
            filters: { wiql: "SELECT [System.Id] FROM WorkItems", top: 10 },
          },
        ],
        pullRequests: [],
      },
      translate,
    );
    expect(customized[0].label).toBe("My recent work");
  });
});

describe("matchingPresetId", () => {
  const base = { text: "", status: "active", reviewer: "", creator: "", wiql: "", top: 50 };

  it("finds the shortcut whose filters the current filters carry", () => {
    expect(matchingPresetId(AZURE_PULL_REQUEST_PRESETS, { ...base, reviewer: "@me" })).toBe("review-requested");
    expect(matchingPresetId(AZURE_PULL_REQUEST_PRESETS, { ...base, creator: "@me" })).toBe("created");
    expect(matchingPresetId(AZURE_PULL_REQUEST_PRESETS, base)).toBe("active");
  });

  it("finds none for custom filters or typed search text", () => {
    expect(matchingPresetId(AZURE_PULL_REQUEST_PRESETS, { ...base, reviewer: "ana@acme.com" })).toBe("");
    const recent = AZURE_WORK_ITEM_PRESETS[0];
    expect(matchingPresetId(AZURE_WORK_ITEM_PRESETS, { ...base, ...recent.filters })).toBe(recent.value);
    expect(matchingPresetId(AZURE_WORK_ITEM_PRESETS, { ...base, ...recent.filters, text: "token" })).toBe("");
  });
});

describe("advancedFilterCount", () => {
  it("counts the pull-request filters hidden under Advanced", () => {
    expect(advancedFilterCount({ creator: "", reviewer: "@me" })).toBe(1);
    expect(advancedFilterCount({ creator: " ", reviewer: "" })).toBe(0);
    expect(advancedFilterCount({ creator: "@me", reviewer: "@me" })).toBe(2);
  });
});
