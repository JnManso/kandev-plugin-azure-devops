---
status: draft
system: integrations
created: 2026-11-11
owners:
  - kandev
---

# Azure DevOps Plugin Board Card Children Requirements

## Overview

The Azure DevOps connector plugin ([requirements](azure-devops-plugin.md))
shows Azure Boards Kanban boards. This capability shows each card's child
work items on the card, as Azure Boards does with its card child list, and
narrows a board to one person's cards.

## Terminology

- **Child:** a work item linked to a card's work item with the
  `System.LinkTypes.Hierarchy-Forward` (parent to child) link, such as an
  epic's features, a feature's user stories, or a user story's tasks.
- **Done child:** a child whose state belongs to the `Completed` state
  category of its work-item type.

## Requirements

### REQ-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-CHILDREN-001: Board card children

**Intent:** Users see what a card breaks down into without leaving the Board:
an epic's features, a feature's stories, a story's tasks. Requested by the
owner on 2026-11-11 after testing the Offers Epics and Features boards on
`contoso`.

#### Acceptance criteria

- **AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-CHILDREN-001.1:** A Board card whose
  work item has children shall show how many of them are done out of the
  total, named by the children's work-item type when they share one. Children
  in the `Removed` state category are not counted or listed. Cards without
  children show no count. The Board's cards shall appear without waiting for
  the counts.
- **AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-CHILDREN-001.2:** Selecting the count
  shall expand the card to list its children below its fields, one line each
  with number, type, title, state, and assignee, in backlog order; selecting
  it again shall collapse the list. A child with children of its own shall
  show their count and expand the same way below its line, level by level
  down to work items without children (amended 2026-11-11 at the owner's
  request). A level's counts are read only when the level above is shown.
- **AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-CHILDREN-001.3:** A child line shall
  open the child's work-item detail and offer the same task actions as a
  work-item result, without opening, expanding, or moving its card. Children
  cannot be dragged.
- **AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-CHILDREN-001.4:** One Board control
  shall expand or collapse the first level of every card with children. The system shall
  remember that choice per board for each user and workspace.
- **AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-CHILDREN-001.5:** On a phone, cards
  shall expand in place with the same lines and actions, without horizontal
  scrolling of the page.
- **AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-CHILDREN-001.6:** When the children
  cannot be read, the Board shall still show its cards, without counts, and
  say that the child items are unavailable. When a board has more children
  than the system reads at once, the Board shall say that some child items
  are not shown.

### REQ-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-CHILDREN-002: Board person filter

**Intent:** Users see only one person's cards on a board, as they already
can on the Sprints taskboard, for example to review their own work on a
Stories or Features board. Requested by the owner on 2026-11-19.

#### Acceptance criteria

- **AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-CHILDREN-002.1:** The Board scope bar
  shall offer a person selector after the board selector, with the options
  and labels of the Sprints person selector: **All people**, **Unassigned**,
  and, by name, each person assigned to a card of the board or to one of its
  cards' direct children.
- **AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-CHILDREN-002.2:** A person shall keep
  the cards assigned to them and the cards with a direct child assigned to
  them; **Unassigned** keeps the cards that, or one of whose direct children,
  have no assignee. Column counts shall count the cards shown. An expanded
  card shall still list all its children with the same done/total count, and
  grandchildren shall not change which cards are shown. When no card matches,
  the Board shall say so and keep the selectors usable.
- **AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-CHILDREN-002.3:** The system shall
  remember the person per board for each user and workspace, and the board
  shall open filtered by it. When the remembered person is not on the board,
  the selector shall show **All people** and every card, without forgetting
  the remembered person.
- **AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-CHILDREN-002.4:** On a phone, the
  Board selectors shall stack full width, and the filter shall apply to the
  column navigator and its counts, without horizontal scrolling of the page.

## Out of scope

- Expanding every level at once, adding or reordering children, and editing
  a child's state from its line (the detail offers it).
- A **Me** option, filtering by grandchildren, and filtering several
  people at once.
- Kandev host changes.

## System design

- [Azure DevOps Plugin Board Card Children System Design](../system-design/azure-devops-plugin-board-children.md)
