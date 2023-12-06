/**
 * Teleport
 * Copyright (C) 2023  Gravitational, Inc.
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU Affero General Public License for more details.
 *
 * You should have received a copy of the GNU Affero General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 */

import React from 'react';

import { ResourceKind } from 'teleport/Discover/Shared';
import { AgentStepComponent } from 'teleport/Discover/types';
import { DiscoverEvent } from 'teleport/services/userEvent';

import { ResourceSpec } from './SelectResource';

type ViewFunction<T> = (t: T) => View[];

export interface ResourceViewConfig<T = any> {
  kind: ResourceKind;
  /**
   * views contain all the possible views for a resource kind.
   * Resources with no sub types will have views defined
   * in a simple View list (eg. kubernetes and servers).
   * ViewFunction is defined instead if a resource can have
   * varying views depending on the resource "sub-type". For
   * example, a database resource can have many sub-types.
   * A aws postgres will contain different views versus a
   * self-hosted postgres.
   */
  views: View[] | ViewFunction<T>;
  wrapper?: (component: React.ReactNode) => React.ReactNode;
  /**
   * shouldPrompt is an optional function that determines if the
   * react-router-dom's Prompt should be invocated on exit or
   * changing route. We can control when to show the prompt
   * depending on what step in the flow a user is in (indicated
   * by "currentStep" param).
   * Not supplying a function is equivalent to always prompting
   * on exit or changing route.
   */
  shouldPrompt?: (currentStep: number, resourceSpec: ResourceSpec) => boolean;
}

export interface View {
  title: string;
  component?: AgentStepComponent;
  hide?: boolean;
  index?: number;
  views?: View[];
  eventName?: DiscoverEvent;
  /**
   * manuallyEmitSuccessEvent is a flag that when true
   * means success events will be sent by the children
   * (current view component) instead of the default
   * which is sent by the parent context.
   */
  manuallyEmitSuccessEvent?: boolean;
}

/**
 * computeViewChildrenSize calculates how many children a view has, without counting the first
 * child. This is because the first child shares the same index with its parent, so we don't
 * need to count it as it's not taking up a new index
 */
export function computeViewChildrenSize(views: View[]) {
  let size = 0;
  for (const view of views) {
    if (view.views) {
      size += computeViewChildrenSize(view.views);
    } else {
      size += 1;
    }
  }

  return size;
}

/**
 * addIndexToViews will recursively loop over the given views, adding an index value to each one
 * The first child shares its index with the parent, as we effectively ignore the fact the parent
 * exists when trying to find the active view by the current step index.
 */
export function addIndexToViews(views: View[], index = 0): View[] {
  const result: View[] = [];

  for (const view of views) {
    const copy = {
      ...view,
      index,
      parent,
    };

    if (view.views) {
      copy.views = addIndexToViews(view.views, index);

      index += computeViewChildrenSize(view.views);
    } else {
      index += 1;
    }

    result.push(copy);
  }

  return result;
}

/**
 * findViewAtIndex will recursively loop views and their children in order to find the deepest
 * match at that index.
 */
export function findViewAtIndex(
  views: View[],
  currentStep: number
): View | null {
  for (const view of views) {
    if (view.views) {
      const result = findViewAtIndex(view.views, currentStep);

      if (result) {
        return result;
      }
    }

    if (currentStep === view.index) {
      return view;
    }
  }
}

/**
 * hasActiveChildren will recursively loop through views and their children in order to find
 * out if there is a view with a matching index to the given `currentStep` value
 * This is because a parent is active as long as its children are active
 */
export function hasActiveChildren(views: View[], currentStep: number) {
  for (const view of views) {
    if (view.index === currentStep) {
      return true;
    }

    if (view.views && hasActiveChildren(view.views, currentStep)) {
      return true;
    }
  }

  return false;
}
