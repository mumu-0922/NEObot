import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { describe, expect, it } from "vitest";

import {
  CHAT_COMPOSER_MIN_CLEARANCE_PX,
  getChatComposerClearance,
  getChatScrollDistanceFromBottom,
  resolveChatScrollFollowOnScroll,
  resolveChatScrollFollowOnWheel,
} from "@/lib/chat/scrollFollow";

describe("chat scroll follow", () => {
  it("pauses immediately when the user wheels upward", () => {
    expect(resolveChatScrollFollowOnWheel(true, -1)).toBe(false);
    expect(resolveChatScrollFollowOnWheel(true, 1)).toBe(true);
  });

  it("pauses on upward scrolling and resumes only at the bottom", () => {
    expect(
      resolveChatScrollFollowOnScroll({
        isFollowing: true,
        previousScrollTop: 900,
        scrollTop: 840,
        distanceFromBottom: 60,
      }),
    ).toBe(false);
    expect(
      resolveChatScrollFollowOnScroll({
        isFollowing: false,
        previousScrollTop: 840,
        scrollTop: 880,
        distanceFromBottom: 80,
      }),
    ).toBe(false);
    expect(
      resolveChatScrollFollowOnScroll({
        isFollowing: false,
        previousScrollTop: 880,
        scrollTop: 920,
        distanceFromBottom: 40,
      }),
    ).toBe(true);
  });

  it("calculates bounded bottom distance and composer clearance", () => {
    expect(
      getChatScrollDistanceFromBottom({
        scrollHeight: 1_200,
        scrollTop: 900,
        clientHeight: 200,
      }),
    ).toBe(100);
    expect(
      getChatScrollDistanceFromBottom({
        scrollHeight: 500,
        scrollTop: 400,
        clientHeight: 200,
      }),
    ).toBe(0);
    expect(getChatComposerClearance(0)).toBe(CHAT_COMPOSER_MIN_CLEARANCE_PX);
    expect(getChatComposerClearance(210.2)).toBe(231);
  });

  it("wires manual pause and measured composer clearance into ChatApp", () => {
    const chatApp = readFileSync(
      resolve(process.cwd(), "src/components/app/ChatApp.tsx"),
      "utf8",
    );

    expect(chatApp).toContain("onWheel={handleMessageWheel}");
    expect(chatApp).toContain("hasWheelMessageScrollIntentRef.current = true");
    expect(chatApp).toContain("onPointerDown={() =>");
    expect(chatApp).toContain("new ResizeObserver(updateComposerClearance)");
    expect(chatApp).toContain("ref={setComposerAreaElement}");
    expect(chatApp).toContain("}, [composerAreaElement]);");
    expect(chatApp).toContain("paddingBottom: `${composerClearance}px`");
    expect(chatApp).not.toContain("messagesEndRef.current?.scrollIntoView");
    expect(chatApp).not.toContain(
      "motion-safe:scroll-smooth scrollbar-overlay",
    );
  });
});
