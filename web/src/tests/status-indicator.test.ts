import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/svelte";
import StatusIndicator from "$lib/components/StatusIndicator.svelte";

describe("StatusIndicator", () => {
  it.each([
    ["online", "online"],
    ["offline", "offline"],
    ["unknown", "unknown"],
    ["sent", "online"],
    ["queued", "idle"],
    ["failed", "offline"],
  ] as const)("shows %s as %s", (status, kind) => {
    render(StatusIndicator, { props: { status, showLabel: true } });
    expect(screen.getByRole("status")).toHaveAttribute("aria-label", kind);
    expect(screen.getByText(kind)).toBeInTheDocument();
  });
});
