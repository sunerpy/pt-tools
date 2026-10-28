import { request } from "./bridge";
export interface NamedOption {
  id: number;
  name: string;
}
export interface MobileOptions {
  sites: { name: string; display_name: string }[];
  downloaders: NamedOption[];
  profiles: NamedOption[];
  libraries: NamedOption[];
  errors?: Record<string, string>;
}
export interface Schedule {
  kind: string;
  interval_minutes?: number;
  cron?: string;
  at?: string;
  every?: number;
  air_delay_hours?: number;
  air_retry_hours?: number;
  air_window_days?: number;
  days?: number[];
}
export interface SubscriptionForm {
  media_type: string;
  tmdb_id: number;
  season: number;
  profile_id: number;
  sites: string[];
  downloader_id: number;
  category: string;
  tags: string;
  save_path: string;
  schedule: Schedule;
  lock_version: boolean;
  start_episode: number;
  follow_next_season: boolean;
  library_id: number;
  upgrade: boolean;
  include_keywords: string;
  exclude_keywords: string;
}
export const mobileApi = {
  options: () => request<MobileOptions>("/api/app/v1/mobile/options"),
  subscription: (id: number) =>
    request<SubscriptionForm & { title: string }>(`/api/app/v1/subscriptions/${id}`),
};
