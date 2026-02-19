import { Prompt } from "@modelcontextprotocol/sdk/types.js";

/**
 * Pre-defined search prompts for common use cases
 * MCP 2025-11-25 specification: Prompts capability
 */
export const SEARCH_PROMPTS: Record<string, Prompt> = {
  "tech-news": {
    name: "tech-news",
    description: "Search for latest technology news and articles",
    arguments: [
      {
        name: "topic",
        description: "Technology topic (e.g., AI, blockchain, cloud computing)",
        required: true,
      },
      {
        name: "time_range",
        description: "Time range for search results",
        required: false,
      },
    ],
  },
  "academic-search": {
    name: "academic-search",
    description: "Search for academic papers and research articles",
    arguments: [
      {
        name: "field",
        description: "Research field or keyword",
        required: true,
      },
      {
        name: "year",
        description: "Publication year (e.g., 2024, 2023)",
        required: false,
      },
    ],
  },
  "recent-updates": {
    name: "recent-updates",
    description: "Search for recent updates on a specific topic",
    arguments: [
      {
        name: "topic",
        description: "Topic to search for recent updates",
        required: true,
      },
    ],
  },
  "how-to": {
    name: "how-to",
    description: "Search for tutorials and how-to guides",
    arguments: [
      {
        name: "task",
        description: "The task or skill to learn",
        required: true,
      },
      {
        name: "level",
        description: "Difficulty level (beginner, intermediate, advanced)",
        required: false,
      },
    ],
  },
};

/**
 * Get prompt template based on prompt name and arguments
 */
export function getPromptTemplate(
  name: string,
  args: Record<string, string>
): string {
  const prompt = SEARCH_PROMPTS[name];
  if (!prompt) {
    throw new Error(`Unknown prompt: ${name}`);
  }

  switch (name) {
    case "tech-news":
      return `Search for latest technology news about "${args.topic}"${
        args.time_range ? ` in the last ${args.time_range}` : ""
      }. Include recent developments, announcements, and industry updates.`;

    case "academic-search":
      return `Search for academic papers and research about "${args.field}"${
        args.year ? ` published in ${args.year}` : ""
      }. Focus on peer-reviewed sources and scholarly articles.`;

    case "recent-updates":
      return `Search for recent updates and news about "${args.topic}". Include the latest developments from the past few days or weeks.`;

    case "how-to":
      return `Search for tutorials and how-to guides about "${args.task}"${
        args.level ? ` for ${args.level} level` : ""
      }. Include step-by-step instructions and practical examples.`;

    default:
      throw new Error(`Unhandled prompt: ${name}`);
  }
}

/**
 * Validate prompt arguments
 */
export function validatePromptArgs(
  promptName: string,
  args: Record<string, string> | undefined
): { valid: boolean; error?: string } {
  const prompt = SEARCH_PROMPTS[promptName];
  if (!prompt) {
    return { valid: false, error: `Unknown prompt: ${promptName}` };
  }

  const providedArgs = args || {};

  // Check required arguments
  for (const arg of prompt.arguments || []) {
    if (arg.required && !providedArgs[arg.name]) {
      return {
        valid: false,
        error: `Missing required argument: ${arg.name}`,
      };
    }
  }

  return { valid: true };
}
