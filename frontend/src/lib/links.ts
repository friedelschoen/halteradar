/*
Copyright (C) 2026 Friedel Schön

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program.  If not, see <http://www.gnu.org/licenses/>.
*/

export function stopURL(stopID?: string | null): string {
    if (!stopID) return "";
    return `/stop/${encodeURIComponent(stopID)}`;
}

export function routeURL(routeID?: string | null): string {
    if (!routeID) return "";
    return `/route/${encodeURIComponent(routeID)}`;
}

export function vehicleURL(
    dataOwner?: string | null,
    vehicleNumber?: string | number | null,
): string {
    if (!dataOwner || !vehicleNumber) return "";
    return `/vehicle/${encodeURIComponent(dataOwner)}/${encodeURIComponent(vehicleNumber)}`;
}

export function blockURL(
    dataOwner?: string | null,
    blockCode?: string | number | null,
): string {
    if (!dataOwner || !blockCode) return "";
    return `/block/${encodeURIComponent(dataOwner)}/${encodeURIComponent(blockCode)}`;
}
