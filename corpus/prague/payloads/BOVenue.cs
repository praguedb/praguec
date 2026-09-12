using MessagePack;

namespace BO.Services.Prague.Models;

[MessagePackObject]
public struct BOVenue
{
    [Key(1)]
    public required long Id { get; set; }
    [Key(2)]
    public required string Name { get; set; }
}
