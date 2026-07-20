export const CHAT_SCROLL_RESUME_DISTANCE_PX = 48;
export const CHAT_COMPOSER_MIN_CLEARANCE_PX = 176;
export const CHAT_COMPOSER_GAP_PX = 20;

interface ChatScrollMetrics {
  scrollHeight: number;
  scrollTop: number;
  clientHeight: number;
}

export function getChatScrollDistanceFromBottom({
  scrollHeight,
  scrollTop,
  clientHeight,
}: ChatScrollMetrics): number {
  return Math.max(0, scrollHeight - scrollTop - clientHeight);
}

export function resolveChatScrollFollowOnScroll({
  isFollowing,
  previousScrollTop,
  scrollTop,
  distanceFromBottom,
}: {
  isFollowing: boolean;
  previousScrollTop: number;
  scrollTop: number;
  distanceFromBottom: number;
}): boolean {
  if (scrollTop < previousScrollTop) return false;
  if (
    scrollTop > previousScrollTop &&
    distanceFromBottom <= CHAT_SCROLL_RESUME_DISTANCE_PX
  ) {
    return true;
  }
  return isFollowing;
}

export function resolveChatScrollFollowOnWheel(
  isFollowing: boolean,
  deltaY: number,
): boolean {
  return deltaY < 0 ? false : isFollowing;
}

export function getChatComposerClearance(height: number): number {
  if (!Number.isFinite(height) || height <= 0) {
    return CHAT_COMPOSER_MIN_CLEARANCE_PX;
  }
  return Math.max(
    CHAT_COMPOSER_MIN_CLEARANCE_PX,
    Math.ceil(height) + CHAT_COMPOSER_GAP_PX,
  );
}
