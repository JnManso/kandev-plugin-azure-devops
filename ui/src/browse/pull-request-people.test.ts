import { describe, expect, it } from "vitest";
import { CURRENT_USER, personPickerOptions } from "./pull-request-people";

const meLabel = (name?: string) => (name ? `Me - ${name}` : "Me");

const people = [
  { id: "ana", displayName: "Ana Silva", uniqueName: "ana@acme.com" },
  { id: "team", displayName: "[Contoso]\\Offers Team" },
];

describe("personPickerOptions", () => {
  it("offers me first, then each person by name", () => {
    expect(personPickerOptions(people, meLabel, "")).toEqual([
      { id: CURRENT_USER, name: "Me" },
      { id: "ana", name: "Ana Silva", description: "ana@acme.com" },
      { id: "team", name: "[Contoso]\\Offers Team", description: undefined },
    ]);
  });

  it("keeps a selected value the list does not name", () => {
    const options = personPickerOptions(people, meLabel, "rui-id");
    expect(options.at(-1)).toEqual({ id: "rui-id", name: "rui-id" });
    expect(personPickerOptions(people, meLabel, CURRENT_USER)).toHaveLength(3);
    expect(personPickerOptions(people, meLabel, "ana")).toHaveLength(3);
  });
});

describe("personPickerOptions with the signed-in identity", () => {
  const me = { id: "ana", displayName: "Ana Silva" };

  it("names me with my pull-request name and email and leaves my own entry out", () => {
    expect(personPickerOptions(people, meLabel, "", { id: "ANA", displayName: "Other display name", uniqueName: "x1@acme.com" })).toEqual([
      { id: CURRENT_USER, name: "Me - Ana Silva", description: "ana@acme.com" },
      { id: "team", name: "[Contoso]\\Offers Team", description: undefined },
    ]);
  });

  it("names me from the identity when the pull requests do not", () => {
    expect(personPickerOptions([], meLabel, "", { id: "rui", displayName: "Rui Costa", uniqueName: "rui@acme.com" })).toEqual([
      { id: CURRENT_USER, name: "Me - Rui Costa", description: "rui@acme.com" },
    ]);
  });

  it("keeps my own entry while a saved filter names me by ID", () => {
    expect(personPickerOptions(people, meLabel, "ANA", me).map((option) => option.id)).toEqual([CURRENT_USER, "ana", "team"]);
  });
});
