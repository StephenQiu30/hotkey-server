export interface PostSummary {
  id: number;
  authorName: string;
  text: string;
  score: number;
}

export function PostFeed({ posts }: { posts: PostSummary[] }) {
  return (
    <ul>
      {posts.map((post) => (
        <li key={post.id}>
          <strong>{post.authorName}</strong>
          <p>{post.text}</p>
          <span>Score: {post.score}</span>
        </li>
      ))}
    </ul>
  );
}
