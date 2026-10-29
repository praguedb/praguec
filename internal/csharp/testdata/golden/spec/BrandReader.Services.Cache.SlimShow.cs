using Common.EventApi.Cache.Prague.Models;
using MessagePack;

namespace BrandReader.Services.Cache;

[MessagePackObject]
public sealed partial class SlimShow
{
    [Key(0)]
    [DataCacheKey]
    public long Id { get; set; }

    [Key(1)]
    public long GenreId { get; set; }

    [Key(2)]
    public long CountryId { get; set; }

    [Key(3)]
    public long TourId { get; set; }

    [Key(4)]
    public long? HeadlinerId { get; set; }

    [Key(6)]
    public Venue? Venue { get; set; }

    [Key(54)]
    public StaticDataRef Genre { get; set; }
}
