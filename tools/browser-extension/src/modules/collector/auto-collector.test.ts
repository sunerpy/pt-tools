import { describe, expect, it } from "vitest";

import {
  extractNexusPHPTorrentId,
  extractOwnUserId,
  findTorrentId,
  userInfoHasDetailRows,
} from "./auto-collector";

describe("extractNexusPHPTorrentId", () => {
  const cases: Array<{ name: string; html: string; expected: string | null }> = [
    {
      name: "prefers the free torrent in a table layout",
      html: `
        <table class="torrents">
          <tr><td><a href="details.php?id=10001">First torrent</a></td></tr>
          <tr><td><a href="details.php?id=10002">Free torrent</a><span class='free'>Free</span></td></tr>
        </table>
      `,
      expected: "10002",
    },
    {
      name: "returns the first torrent in a table layout without a free marker",
      html: `
        <table class="torrents">
          <tr><td><a href="details.php?id=20001">First torrent</a></td></tr>
          <tr><td><a href="details.php?id=20002">Second torrent</a></td></tr>
        </table>
      `,
      expected: "20001",
    },
    {
      name: "finds a free torrent in the div layout used by cspt",
      html: `
        <div class="torrents">
          <div class="torrent-cat"><img alt="Movies" /></div>
          <div class="torrent-title">
            <a href="details.php?id=30001&amp;hit=1">First torrent</a>
          </div>
          <div class="torrent-info"><a href="details.php?id=30001&amp;dllist=1">4</a></div>
          <div class="torrent-cat"><img alt="TV" /></div>
          <div class="torrent-title">
            <a href="details.php?id=30002&amp;hit=1">Free torrent</a>
            <img class="pro_free" alt="Free" />
          </div>
        </div>
      `,
      expected: "30002",
    },
    {
      name: "returns the first torrent in a div layout without a free marker",
      html: `
        <div class="torrents">
          <div class="torrent-cat"><img alt="Movies" /></div>
          <div class="torrent-title">
            <a href="details.php?id=40001&amp;hit=1">First torrent</a>
          </div>
          <div class="torrent-title">
            <a href="details.php?id=40002&amp;hit=1">Second torrent</a>
          </div>
        </div>
      `,
      expected: "40001",
    },
    {
      name: "does not treat a userdetails link as a torrent link",
      html: `<table><tr><td><a href="userdetails.php?id=12345">Anonymous user</a></td></tr></table>`,
      expected: null,
    },
    {
      name: "returns null for empty HTML",
      html: "",
      expected: null,
    },
    {
      name: "returns null for empty and no-match HTML",
      html: `<div class="torrents">No torrent links</div>`,
      expected: null,
    },
  ];

  it.each(cases)("$name", ({ html, expected }) => {
    expect(extractNexusPHPTorrentId(html)).toBe(expected);
  });
});

describe("findTorrentId", () => {
  // Mirrors the piggo capture from issue #532: every torrent was 50% off and the
  // site had no free torrent at all.
  const fiftyPercentOnly = `
    <table class="torrents">
      <tr><td><a href="details.php?id=57052">Half off</a><img class="pro_50pctdown" alt="50%" /></td></tr>
      <tr><td><a href="details.php?id=57053">Half off too</a><img class="pro_50pctdown" alt="50%" /></td></tr>
    </table>
  `;

  const mixedPromotions = `
    <table class="torrents">
      <tr><td><a href="details.php?id=1">Free</a><img class="pro_free" alt="Free" /></td></tr>
      <tr><td><a href="details.php?id=2">Half</a><img class="pro_50pctdown" alt="50%" /></td></tr>
      <tr><td><a href="details.php?id=3">Plain</a></td></tr>
    </table>
  `;

  it("reports no free torrent instead of silently returning another one", () => {
    expect(findTorrentId(fiftyPercentOnly, "free")).toBeNull();
  });

  it("still returns a torrent when the caller accepts a fallback", () => {
    expect(findTorrentId(fiftyPercentOnly, "freePreferred")).toBe("57052");
  });

  it("finds no promotion-free torrent when every row carries a promotion", () => {
    expect(findTorrentId(fiftyPercentOnly, "nopromo")).toBeNull();
  });

  it("skips promoted rows when looking for a promotion-free torrent", () => {
    expect(findTorrentId(mixedPromotions, "nopromo")).toBe("3");
  });

  it("picks the free row when one exists", () => {
    expect(findTorrentId(mixedPromotions, "free")).toBe("1");
  });
});

describe("extractOwnUserId", () => {
  it("reads the uid from a table-based info_block", () => {
    // piggo renders info_block as a <table>, which the previous <div>-only
    // regex could never match.
    const html = `<table id="info_block" width="100%"><tr><td>
      <a href="https://piggo.me/userdetails.php?id=21646" class='User_Name'><b>demo</b></a>
    </td></tr></table>`;
    expect(extractOwnUserId(html)).toBe("21646");
  });

  it("reads the uid from a div-based info_block", () => {
    const html = `<div id="info_block"><span class="medium">
      <a href="userdetails.php?id=126666" class='VeteranUser_Name'><b>demo</b></a>
    </span></div>`;
    expect(extractOwnUserId(html)).toBe("126666");
  });

  it("falls back to the nav block", () => {
    const html = `<div id="userbar"><a href="userdetails.php?id=777">demo</a></div>`;
    expect(extractOwnUserId(html)).toBe("777");
  });

  it("returns null on a torrent listing instead of guessing a busy uploader", () => {
    // Regression for issue #532: the old frequency fallback picked the most
    // linked uploader and the collector captured a stranger's profile page.
    const listing = `
      <table class="torrents">
        <tr><td><a href="userdetails.php?id=25096">Nature</a></td></tr>
        <tr><td><a href="userdetails.php?id=25096">Nature</a></td></tr>
        <tr><td><a href="userdetails.php?id=25096">Nature</a></td></tr>
        <tr><td><a href="userdetails.php?id=24283">Other</a></td></tr>
      </table>
    `;
    expect(extractOwnUserId(listing)).toBeNull();
  });

  it("returns null when no user link is present", () => {
    expect(extractOwnUserId(`<div class="torrents">nothing</div>`)).toBeNull();
  });
});

describe("userInfoHasDetailRows", () => {
  it("accepts a profile page that exposes rowhead rows", () => {
    const html = `<table><tr><td class="rowhead nowrap">最近动向</td><td class="rowfollow">2026-09-07</td></tr></table>`;
    expect(userInfoHasDetailRows(html)).toBe(true);
  });

  it("rejects a privacy-protected profile page", () => {
    // What issue #532 actually captured: a stranger's page with no detail rows.
    const html = `<div class="main">该用户的个人信息受保护</div>`;
    expect(userInfoHasDetailRows(html)).toBe(false);
  });
});
