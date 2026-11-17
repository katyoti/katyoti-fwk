// SPDX-License-Identifier: Apache-2.0
export interface Type<T = any> extends Function {
    new (...args: any[]): T;
}

export interface Abstract<T> extends Function {
    prototype: T;
}

export type ArrayOrString = string | string[];

//= =========================================
// JSON global types
//= =========================================

//* * Basic string types to allow precise definition of expected string type */
export type JSONString = string;
export type URLString = string;
export type ISODateString = string;
export type ISODateTimeString = string;
export type EmailAddressString = string;
export type HtmlString = string;
export type MarkdownString = string;
export type Base64String = string;
export type PathString = string;
export type RFC3339String = string;
export type UUIDString = string;
export type DataRecord<T = any> = Record<string, T>;
export type DataArray<T = any> = Array<DataRecord<T>>;


/**
 * A sleep-compatible time string such as `"1h30m15s"` that can be sent to
 * Inngest to sleep for a given amount of time.
 *
 * This type includes an empty string too, so make sure to exclude that via
 * `Exclude<TimeStr, "">` if you don't want to allow empty strings.
 *
 * @public
 */
export type DurationStr = `${`${number}w` | ''}${`${number}d` | ''}${`${number}h` | ''}${`${number}m` | ''}${`${number}s` | ''}`;


