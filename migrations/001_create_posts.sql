create table if not exists posts (
  id varchar(32) primary key,
  title varchar(255) not null,
  slug varchar(255) not null unique,
  excerpt text not null,
  markdown_path varchar(512) not null,
  rendered_html_path varchar(512) not null,
  cover_image_path varchar(512),
  status varchar(20) not null check (status in ('draft', 'published', 'archived')),
  author_id varchar(255),
  seo_title varchar(255),
  seo_description text,
  published_at datetime(6),
  created_at datetime(6) not null default current_timestamp(6),
  updated_at datetime(6) not null default current_timestamp(6)
);

create index posts_status_updated_at_idx on posts (status, updated_at desc);
create index posts_status_published_at_idx on posts (status, published_at desc);
